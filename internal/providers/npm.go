package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v62/github"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type NpmPackage struct {
	ID          string                       `json:"_id"`
	Name        string                       `json:"name"`
	DistTags    map[string]string            `json:"dist-tags"`
	Versions    map[string]NpmPackageVersion `json:"versions"`
	Time        map[string]string            `json:"time"`
	Description string                       `json:"description"`
	Author      map[string]interface{}       `json:"author"`
	Homepage    string                       `json:"homepage"`
	Repository  RepositoryInfo               `json:"repository"`
	Bugs        BugsInfo                     `json:"bugs"`
}

type NpmPackageVersion struct {
	Name          string                 `json:"name"`
	Version       string                 `json:"version"`
	ID            string                 `json:"_id"`
	NodeVersion   string                 `json:"_nodeVersion"`
	NpmVersion    string                 `json:"_npmVersion"`
	Dist          DistInfo               `json:"dist"`
	NpmUser       UserInfo               `json:"_npmUser"`
	Description   string                 `json:"description"`
	Main          string                 `json:"main"`
	Author        map[string]interface{} `json:"author"`
	GitHead       string                 `json:"gitHead"`
	Directories   map[string]interface{} `json:"directories"`
	Repository    RepositoryInfo         `json:"repository"`
	Homepage      string                 `json:"homepage"`
	Bugs          BugsInfo               `json:"bugs"`
	HasShrinkwrap bool                   `json:"_hasShrinkwrap"`
	Readme        string                 `json:"readme"`
}

type DistInfo struct {
	Integrity string `json:"integrity"`
	Shasum    string `json:"shasum"`
	Tarball   string `json:"tarball"`
}

type UserInfo struct {
	Name string `json:"name"`
}

type RepositoryInfo struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type BugsInfo struct {
	URL string `json:"url"`
}

type NPMProvider struct {
	BaseProvider
}

func NewNPMProvider(logger *zap.Logger, packageType string) Provider {
	return &NPMProvider{
		BaseProvider: NewBaseProvider(packageType, "", "", false),
	}
}

func (p *NPMProvider) Connect(logger *zap.Logger) error {
	return nil
}

func (p *NPMProvider) FetchPackageFiles(logger *zap.Logger, owner, repository, packageType, packageName, version string, metadata *github.PackageMetadata) ([]string, ResultState, error) {
	logger.Info("Loading package files from NPM package registry")
	fetchUrl, err := p.GetFetchUrl(logger, owner, packageName, version)
	if err != nil {
		return nil, Failed, err
	}
	client := &http.Client{}
	req, err := http.NewRequest("GET", fetchUrl, nil)
	if err != nil {
		return nil, Failed, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", viper.GetString("GHMPKG_SOURCE_TOKEN")))
	resp, err := client.Do(req)
	if err != nil {
		return nil, Failed, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, Failed, fmt.Errorf("failed to fetch package %s, status: %d, message: %s", fetchUrl, resp.StatusCode, resp.Status)
	}
	// print json response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, Failed, err
	}
	var npmPackage NpmPackage
	if err := json.Unmarshal(body, &npmPackage); err != nil {
		return nil, Failed, err
	}
	tarballUrl, err := url.Parse(npmPackage.Versions[version].Dist.Tarball)
	if err != nil {
		return nil, Failed, err
	}
	filename := path.Base(tarballUrl.Path)
	var filenames []string
	filenames = append(filenames, filename)
	return filenames, Success, nil
}

func (p *NPMProvider) Export(logger *zap.Logger, owner string, content interface{}) error {
	return p.BaseProvider.Export(logger, owner, content)
}

func (p *NPMProvider) Download(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
	downloadedFilename := fmt.Sprintf("%s-%s.tgz", packageName, version)
	return p.downloadPackage(
		logger, owner, repository, packageType, packageName, version, filename, &downloadedFilename,
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

func (p *NPMProvider) Rename(logger *zap.Logger, filename string) error {
	// Read the file
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read package.json: %w", err)
	}

	// Replace the organization name in the content
	oldScope := fmt.Sprintf("@%s/", viper.GetString("GHMPKG_SOURCE_ORGANIZATION"))
	newScope := fmt.Sprintf("@%s/", viper.GetString("GHMPKG_TARGET_ORGANIZATION"))
	newContent := strings.Replace(string(content), oldScope, newScope, -1)

	// Write back to file
	err = os.WriteFile(filename, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}

	return nil
}

func (p *NPMProvider) Upload(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
	return p.uploadPackage(
		logger, owner, repository, packageType, packageName, version, filename,
		func() (string, error) {
			return p.GetUploadUrl(logger, owner, repository, packageName, version, filename)
		},
		func(uploadUrl, packageDir string) (ResultState, error) {
			npmrcPath := filepath.Join(packageDir, ".npmrc")
			tgz := fmt.Sprintf("%s-%s.tgz", packageName, version)

			// Create .npmrc content
			npmrcContent := fmt.Sprintf("//npm.pkg.github.com/:_authToken=%s\nregistry=https://npm.pkg.github.com/%s",
				viper.GetString("GHMPKG_TARGET_TOKEN"), owner)

			// Write .npmrc file
			if err := os.WriteFile(npmrcPath, []byte(npmrcContent), 0644); err != nil {
				return Failed, fmt.Errorf("failed to write .npmrc: %w", err)
			}

			// Extract the tgz file
			cmd := exec.Command("tar", "-xzf", tgz)
			cmd.Dir = packageDir
			if err := cmd.Run(); err != nil {
				return Failed, fmt.Errorf("failed to extract package: %w", err)
			}

			packageJson := filepath.Join(packageDir, "package", "package.json")
			if err := p.Rename(logger, packageJson); err != nil {
				return Failed, fmt.Errorf("failed to rename package.json: %w", err)
			}

			// Run npm publish
			publishCmd := exec.Command("npm", "publish", "--verbose", "--ignore-scripts", "--userconfig", npmrcPath)
			publishCmd.Dir = filepath.Join(packageDir, "package")
			publishCmd.Env = append(os.Environ(), "HTTPS_PROXY=")

			// Capture output to npmlog file
			logFile, err := os.Create(filepath.Join(packageDir, "npmlog"))
			if err != nil {
				return Failed, fmt.Errorf("failed to create log file: %w", err)
			}
			defer logFile.Close()

			publishCmd.Stdout = logFile
			publishCmd.Stderr = logFile

			if err := publishCmd.Run(); err != nil {
				return Failed, fmt.Errorf("failed to publish package: %w", err)
			}

			return Success, nil
		},
	)
}

func (p *NPMProvider) GetFetchUrl(logger *zap.Logger, owner, packageName, version string) (string, error) {
	fetchUrl := *p.SourceRegistryUrl
	fetchUrl.Path = path.Join(fetchUrl.Path, fmt.Sprintf("@%s", owner), packageName)
	return fetchUrl.String(), nil
}

func (p *NPMProvider) GetDownloadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error) {
	downloadUrl := *p.SourceRegistryUrl
	downloadUrl.Path = path.Join(downloadUrl.Path, "download", fmt.Sprintf("@%s", owner), packageName, version, filename)
	return downloadUrl.String(), nil
}

func (p *NPMProvider) GetUploadUrl(logger *zap.Logger, owner, repository, packageName, version string, filename string) (string, error) {
	uploadUrl := *p.TargetRegistryUrl
	uploadUrl.Path = path.Join(uploadUrl.Path, fmt.Sprintf("@%s", owner), repository, packageName, version, filename)
	return uploadUrl.String(), nil
}
