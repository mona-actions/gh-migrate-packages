package providers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/go-github/v62/github"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// MavenProvider handles Maven package operations between registries.
// It implements the Provider interface for Maven-specific package handling,
// supporting download and upload of Maven artifacts.
type MavenProvider struct {
	BaseProvider
	httpClient   *http.Client
	client       *githubv4.Client
	ctx          context.Context
	packageFiles []PackageNode
}

// Constructor
// ----------

// NewMavenProvider creates a new instance of MavenProvider
func NewMavenProvider(logger *zap.Logger, packageType string) Provider {
	return &MavenProvider{
		BaseProvider: NewBaseProvider(packageType, "", "", false),
	}
}

// Core Operations
// --------------

// Connect initializes the provider connection
// Currently a no-op for Maven provider
func (p *MavenProvider) Connect(logger *zap.Logger) error {
	return nil
}

// FetchPackageFiles retrieves package files information from GitHub GraphQL API
func (p *MavenProvider) FetchPackageFiles(logger *zap.Logger, owner, repository, packageType, packageName, version string, metadata *github.PackageMetadata) ([]string, ResultState, error) {
	if p.packageFiles == nil || len(p.packageFiles) == 0 {
		packageFiles, _, err := FetchFromGraphQL(logger, owner, viper.GetString("GHMPKG_SOURCE_TOKEN"), string(p.PackageType))
		if err != nil {
			return nil, Failed, err
		}
		p.packageFiles = packageFiles
	}

	var filenames []string
	for _, cachedPkg := range p.packageFiles {
		if string(cachedPkg.Name) != packageName {
			continue
		}
		for _, cachedVersion := range cachedPkg.Versions.Nodes {
			if string(cachedVersion.Version) != version {
				continue
			}
			for _, file := range cachedVersion.Files.Nodes {
				filenames = append(filenames, string(file.Name))
			}
		}
	}

	return filenames, Success, nil
}

// Download retrieves a Maven artifact from the source registry
func (p *MavenProvider) Download(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
	return p.downloadPackage(
		logger, owner, repository, packageType, packageName, version, filename, nil,
		// URL generator function
		func() (string, error) {
			return p.GetDownloadUrl(logger, owner, repository, packageName, version, filename)
		},
		// Download function
		func(downloadUrl, outputPath string) (ResultState, error) {
			if err := utils.DownloadFile(downloadUrl, outputPath, viper.GetString("GHMPKG_SOURCE_TOKEN")); err != nil {
				return Failed, err
			}
			return Success, nil
		},
	)
}

// Rename processes Maven-specific files to update organization references
func (p *MavenProvider) Rename(logger *zap.Logger, repository, packageName, version, filename string) error {
	// Skip if source and target organizations are the same
	if p.CheckOrganizationsMatch(logger) {
		return nil
	}

	// Check if the file is a pom.xml or any .pom file
	if !strings.HasSuffix(filename, "pom.xml") && !strings.HasSuffix(filename, ".pom") {
		logger.Debug("File is not a pom file, skipping",
			zap.String("filename", filename))
		return nil
	}

	// Read the file content
	content, err := os.ReadFile(filename)
	if err != nil {
		logger.Warn("Failed to read pom file",
			zap.String("filename", filename),
			zap.Error(err))
		return nil // Continue with warning
	}

	// Create the search and replace strings
	sourceUrl := fmt.Sprintf("https://maven.pkg.github.com/%s/packages", viper.GetString("GHMPKG_SOURCE_ORGANIZATION"))
	targetUrl := fmt.Sprintf("https://maven.pkg.github.com/%s/packages", viper.GetString("GHMPKG_TARGET_ORGANIZATION"))

	// Replace the content
	newContent := strings.ReplaceAll(string(content), sourceUrl, targetUrl)

	// Write the file back
	if err := os.WriteFile(filename, []byte(newContent), 0644); err != nil {
		logger.Warn("Failed to write updated pom file",
			zap.String("filename", filename),
			zap.Error(err))
		return nil // Continue with warning
	}

	logger.Info("Successfully updated organization reference in file",
		zap.String("filename", filename),
		zap.String("sourceOrg", viper.GetString("GHMPKG_SOURCE_ORGANIZATION")),
		zap.String("targetOrg", viper.GetString("GHMPKG_TARGET_ORGANIZATION")))

	return nil
}

// Upload sends a Maven artifact to the target registry
func (p *MavenProvider) Upload(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {

	// Create a semaphore with size 5 to limit concurrent uploads
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)

	// Start upload in goroutine
	resultChan := make(chan struct {
		state ResultState
		err   error
	}, 1)

	go func() {
		sem <- struct{}{}        // Acquire semaphore
		defer func() { <-sem }() // Release semaphore

		state, err := p.uploadPackage(
			logger, owner, repository, packageType, packageName, version, filename,
			func() (string, error) {
				return p.GetUploadUrl(logger, owner, repository, packageName, version, filename)
			},
			func(uploadUrl, packageDir string) (ResultState, error) {
				inputPath := filepath.Join(packageDir, filename)
				uploadPackageUrl, err := p.GetUploadUrl(logger, owner, repository, packageName, version, filename)
				if err != nil {
					return Failed, err
				}
				logger.Info("Uploading file", zap.String("url", uploadPackageUrl))

				if err := p.Rename(logger, repository, packageName, version, inputPath); err != nil {
					logger.Error("Failed to execute rename operation", zap.Error(err))
					// Continue with upload even if rename fails
				}

				response, err := utils.UploadFile(uploadPackageUrl, inputPath, viper.GetString("GHMPKG_TARGET_TOKEN"))
				if err != nil {
					return Failed, err
				}

				if response.StatusCode == http.StatusConflict {
					return Skipped, nil
				} else if response.StatusCode > 299 {
					return Failed, fmt.Errorf("error uploading file: %s", filename)
				}
				return Success, nil
			},
		)
		resultChan <- struct {
			state ResultState
			err   error
		}{state, err}
	}()

	// Wait for result
	result := <-resultChan
	return result.state, result.err
}

// Batch Operations
// ---------------

// UploadBatch handles concurrent upload of multiple Maven artifacts
func (p *MavenProvider) UploadBatch(logger *zap.Logger, owner, repository, packageType, packageName, version string, filenames []string) ([]ResultState, error) {
	const maxConcurrent = 5
	results := make([]ResultState, len(filenames))
	errChan := make(chan error, len(filenames))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent)

	for i, filename := range filenames {
		wg.Add(1)
		go func(idx int, fname string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			state, err := p.Upload(logger, owner, repository, packageType, packageName, version, fname)
			if err != nil {
				errChan <- err
				return
			}
			results[idx] = state
		}(i, filename)
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// URL Generation
// -------------

// GetDownloadUrl generates the URL for downloading a Maven artifact
func (p *MavenProvider) GetDownloadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error) {
	downloadUrl := *p.SourceRegistryUrl
	downloadUrl.Path = path.Join(downloadUrl.Path, viper.GetString("GHMPKG_SOURCE_ORGANIZATION"), repository, packageName, version, filename)
	return downloadUrl.String(), nil
}

// GetUploadUrl generates the URL for uploading a Maven artifact
func (p *MavenProvider) GetUploadUrl(logger *zap.Logger, owner, repository, packageName, version string, filename string) (string, error) {
	uploadUrl := *p.TargetRegistryUrl
	uploadUrl.Path = path.Join(uploadUrl.Path, viper.GetString("GHMPKG_TARGET_ORGANIZATION"), repository, packageName, version, filename)
	return uploadUrl.String(), nil
}

// Required Interface Methods
// ------------------------

// Export implements the Provider interface Export method
func (p *MavenProvider) Export(logger *zap.Logger, owner string, content interface{}) error {
	return p.BaseProvider.Export(logger, owner, content)
}
