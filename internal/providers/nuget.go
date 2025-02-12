package providers

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/google/go-github/v62/github"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type NugetProvider struct {
	BaseProvider
}

func NewNugetProvider(logger *zap.Logger, packageType string) Provider {
	return &NugetProvider{
		BaseProvider: NewBaseProvider(packageType, "", "", false),
	}
}

func (p *NugetProvider) Connect(logger *zap.Logger) error {
	return nil
}

func (p *NugetProvider) FetchPackageFiles(logger *zap.Logger, owner, repository, packageType, packageName, version string, metadata *github.PackageMetadata) ([]string, ResultState, error) {
	logger.Info("Loading package files from Nuget package registry")
	var filenames []string
	filenames = append(filenames, fmt.Sprintf("%s-%s.nupkg", packageName, version))
	return filenames, Success, nil
}

func (p *NugetProvider) Export(logger *zap.Logger, owner string, content interface{}) error {
	return p.BaseProvider.Export(logger, owner, content)
}

func (p *NugetProvider) Download(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
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

func (p *NugetProvider) Rename(logger *zap.Logger, filename string) error {
	zipCmd := exec.Command("zip", "-d", filename, "_rels/.rels", "\\[Content_Types\\].xml")
	if err := zipCmd.Run(); err != nil {
		if err.Error() == "exit status 12" {
			// ignore the error if the files are not found
			logger.Info("No files to remove from zip archive")
		} else {
			return fmt.Errorf("failed to remove files from %s: %w", filename, err)
		}
	}
	return nil
}

func (p *NugetProvider) Upload(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
	return p.uploadPackage(
		logger, owner, repository, packageType, packageName, version, filename,
		func() (string, error) {
			return p.GetUploadUrl(logger, owner, repository, packageName, version, filename)
		},
		func(uploadUrl, packageDir string) (ResultState, error) {
			nupkg := filepath.Join(packageDir, fmt.Sprintf("%s-%s.nupkg", packageName, version))

			if err := p.Rename(logger, nupkg); err != nil {
				return Failed, fmt.Errorf("failed to rename %s: %w", nupkg, err)
			}

			uploadUrl, err := p.GetUploadUrl(logger, owner, repository, packageName, version, filename)
			if err != nil {
				logger.Error("Error getting upload URL", zap.Error(err))
				return Failed, err
			}
			// Run nuget publish
			pushCmd := exec.Command("./tool/gpr", "push", nupkg, "--repository", uploadUrl, "-k", viper.GetString("GHMPKG_TARGET_TOKEN"))

			// // Capture output to nugetlog file
			logFile, err := os.Create(filepath.Join(packageDir, "nugetlog"))
			if err != nil {
				return Failed, fmt.Errorf("failed to create log file: %w", err)
			}
			defer logFile.Close()

			pushCmd.Stdout = logFile
			pushCmd.Stderr = logFile

			if err := pushCmd.Run(); err != nil {
				return Failed, fmt.Errorf("failed to publish package: %w", err)
			}

			return Success, nil
		},
	)
}

func (p *NugetProvider) GetFetchUrl(logger *zap.Logger, owner, packageName, version string) (string, error) {
	fetchUrl := *p.SourceRegistryUrl
	fetchUrl.Path = path.Join(fetchUrl.Path, owner, "download", packageName, version)
	return fetchUrl.String(), nil
}

func (p *NugetProvider) GetDownloadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error) {
	downloadUrl := *p.SourceRegistryUrl
	downloadUrl.Path = path.Join(downloadUrl.Path, owner, "download", packageName, version, filename)
	return downloadUrl.String(), nil
}

func (p *NugetProvider) GetUploadUrl(logger *zap.Logger, owner, repository, packageName, version string, filename string) (string, error) {
	uploadUrl := *p.TargetHostnameUrl
	uploadUrl.Path = path.Join(uploadUrl.Path, owner, repository)
	return uploadUrl.String(), nil
}
