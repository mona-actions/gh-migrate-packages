package providers

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"path/filepath"

	"github.com/google/go-github/v62/github"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type MavenProvider struct {
	BaseProvider
	httpClient   *http.Client
	client       *githubv4.Client
	ctx          context.Context
	packageFiles []PackageNode
}

func NewMavenProvider(logger *zap.Logger, packageType string) Provider {
	return &MavenProvider{
		BaseProvider: BaseProvider{
			PackageType:       packageType,
			SourceRegistryUrl: utils.ParseUrl(fmt.Sprintf("https://%s.pkg.%s/", packageType, viper.GetString("GHMPKG_SOURCE_HOSTNAME"))),
			TargetRegistryUrl: utils.ParseUrl(fmt.Sprintf("https://%s.pkg.%s/", packageType, viper.GetString("GHMPKG_TARGET_HOSTNAME"))),
			SourceHostnameUrl: utils.ParseUrl(fmt.Sprintf("https://%s/", viper.GetString("GHMPKG_SOURCE_HOSTNAME"))),
			TargetHostnameUrl: utils.ParseUrl(fmt.Sprintf("https://%s/", viper.GetString("GHMPKG_TARGET_HOSTNAME"))),
		},
	}
}

func (p *MavenProvider) Connect(logger *zap.Logger) error {
	return nil
}

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

func (p *MavenProvider) Export(logger *zap.Logger, owner string, content interface{}) error {
	return p.BaseProvider.Export(logger, owner, content)
}

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

func (p *MavenProvider) Upload(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
	return p.uploadPackage(
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
			response, err := utils.UploadFile(uploadPackageUrl, inputPath, viper.GetString("GHMPKG_TARGET_TOKEN"))
			if err != nil {
				// general errors
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
}

func (p *MavenProvider) GetDownloadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error) {
	downloadUrl := *p.SourceRegistryUrl
	downloadUrl.Path = path.Join(downloadUrl.Path, viper.GetString("GHMPKG_SOURCE_ORGANIZATION"), repository, packageName, version, filename)
	return downloadUrl.String(), nil
}

func (p *MavenProvider) GetUploadUrl(logger *zap.Logger, owner, repository, packageName, version string, filename string) (string, error) {
	uploadUrl := *p.TargetRegistryUrl
	uploadUrl.Path = path.Join(uploadUrl.Path, viper.GetString("GHMPKG_TARGET_ORGANIZATION"), repository, packageName, version, filename)
	return uploadUrl.String(), nil
}
