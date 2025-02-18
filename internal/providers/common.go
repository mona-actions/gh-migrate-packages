package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v62/github"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

type DownloadCallback func(string, string) error

var providerLookup = map[string]func(*zap.Logger, string) Provider{
	"container": NewContainerProvider,
	"maven":     NewMavenProvider,
	"npm":       NewNPMProvider,
	"rubygems":  NewRubyGemsProvider,
	"nuget":     NewNugetProvider,
}

func NewProvider(logger *zap.Logger, packageType string) (Provider, error) {
	if providerFunc, ok := providerLookup[packageType]; !ok {
		return nil, errors.New(fmt.Sprintf("provider not found: %s", packageType))
	} else {
		return providerFunc(logger, packageType), nil
	}
}

func newHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{}
	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &http.Client{Transport: transport}, nil
}

func FetchFromGraphQL(logger *zap.Logger, owner, token, packageType string) ([]PackageNode, ResultState, error) {
	logger.Info("Loading package files from GitHub GraphQL API")
	var allPackages []PackageNode
	packagesAfter := (*githubv4.String)(nil)
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	var httpProxy = viper.GetString("HTTPS_PROXY")
	if httpProxy == "" {
		httpProxy = viper.GetString("HTTP_PROXY")
	}
	httpClient, err := newHTTPClient(viper.GetString("HTTPS_PROXY"))
	if err != nil {
		return nil, Failed, err
	}
	oauth2Ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	oauth2Client := oauth2.NewClient(oauth2Ctx, tokenSource)
	client := githubv4.NewClient(oauth2Client)
	ctx := context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)

	for {
		var query Query
		variables := map[string]interface{}{
			"owner":         githubv4.String(owner),
			"packageType":   githubv4.PackageType(strings.ToUpper(packageType)),
			"packagesFirst": githubv4.Int(10),
			"packagesAfter": packagesAfter,
			"versionsFirst": githubv4.Int(10),
			"versionsAfter": (*githubv4.String)(nil),
			"filesFirst":    githubv4.Int(10),
			"filesAfter":    (*githubv4.String)(nil),
		}

		err := client.Query(ctx, &query, variables)
		if err != nil {
			return nil, Failed, fmt.Errorf("error querying packages: %w", err)
		}

		for _, pkg := range query.Organization.Packages.Nodes {

			// Skip deleted packages
			if strings.HasPrefix(string(pkg.Name), "deleted_") {
				continue
			}

			var allVersions []VersionNode
			versionsAfter := (*githubv4.String)(nil)

			for {
				versionVariables := map[string]interface{}{
					"packageID":     githubv4.ID(pkg.ID),
					"versionsFirst": githubv4.Int(10),
					"versionsAfter": versionsAfter,
					"filesFirst":    githubv4.Int(10),
					"filesAfter":    (*githubv4.String)(nil),
				}

				var versionQuery VersionQuery

				err := client.Query(ctx, &versionQuery, versionVariables)
				if err != nil {
					return nil, Failed, fmt.Errorf("error querying versions: %w", err)
				}

				for _, version := range versionQuery.Node.Package.Versions.Nodes {
					var allFiles []FileNode
					filesAfter := (*githubv4.String)(nil)

					for {
						fileVariables := map[string]interface{}{
							"versionID":  githubv4.ID(version.ID),
							"filesFirst": githubv4.Int(10),
							"filesAfter": filesAfter,
						}

						var fileQuery FileQuery
						err := client.Query(ctx, &fileQuery, fileVariables)
						if err != nil {
							return nil, Failed, fmt.Errorf("error querying files: %w", err)
						}

						allFiles = append(allFiles, fileQuery.Node.PackageVersion.Files.Nodes...)

						if !fileQuery.Node.PackageVersion.Files.PageInfo.HasNextPage {
							break
						}
						filesAfter = &fileQuery.Node.PackageVersion.Files.PageInfo.EndCursor
					}

					version.Files.Nodes = allFiles
					allVersions = append(allVersions, version)
				}

				if !versionQuery.Node.Package.Versions.PageInfo.HasNextPage {
					break
				}
				versionsAfter = &versionQuery.Node.Package.Versions.PageInfo.EndCursor
			}

			pkg.Versions.Nodes = allVersions
			allPackages = append(allPackages, pkg)
		}

		if !query.Organization.Packages.PageInfo.HasNextPage {
			break
		}
		packagesAfter = &query.Organization.Packages.PageInfo.EndCursor
	}

	// Cache(fmt.Sprintf("%s-%s-packages.json", owner, strings.ToLower(packageType)), allPackages)
	return allPackages, Success, nil
}

func (p *BaseProvider) downloadPackage(
	logger *zap.Logger,
	owner, repository, packageType, packageName, version, filename string,
	downloadedFilename *string,
	getUrl func() (string, error),
	download func(string, string) (ResultState, error),
) (ResultState, error) {
	if downloadedFilename == nil {
		downloadedFilename = &filename
	}
	outputPath := filepath.Join("migration-packages", "packages", owner, packageType, packageName, version, *downloadedFilename)

	if utils.FileExists(outputPath) {
		logger.Warn("File already exists", zap.String("outputPath", outputPath))
		return Skipped, nil
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		logger.Error("Failed to create directories",
			zap.String("package", packageName),
			zap.String("version", version),
			zap.Error(err))
		return Failed, err
	}

	downloadUrl, err := getUrl()
	if err != nil {
		logger.Error("Error getting download URL",
			zap.String("package", packageName),
			zap.String("version", version),
			zap.Error(err))
		return Failed, err
	}

	logger.Info("Downloading file", zap.String("url", downloadUrl))
	result, err := download(downloadUrl, outputPath)
	if err != nil {
		logger.Error("Error downloading file",
			zap.String("package", packageName),
			zap.String("version", version),
			zap.Error(err))
		return Failed, err
	}

	if result == Skipped {
		logger.Info("File already exists", zap.String("outputPath", outputPath))
	} else {
		logger.Info("Successfully downloaded file", zap.String("outputPath", outputPath))
	}
	return result, nil
}

func (p *BaseProvider) uploadPackage(
	logger *zap.Logger,
	owner, repository, packageType, packageName, version, filename string,
	getUrl func() (string, error),
	upload func(string, string) (ResultState, error),
) (ResultState, error) {
	var packageDir string
	if packageType == "container" {
		parts := strings.Split(filename, ":")
		tag := parts[1]
		packageDir = filepath.Join("migration-packages", "packages", viper.GetString("GHMPKG_SOURCE_ORGANIZATION"), packageType, packageName, tag)
	} else {
		packageDir = filepath.Join("migration-packages", "packages", viper.GetString("GHMPKG_SOURCE_ORGANIZATION"), packageType, packageName, version)
	}

	if !utils.FileExists(packageDir) {
		logger.Warn("Package directory does not exist", zap.String("packageDir", packageDir))
		return Skipped, nil
	}

	uploadUrl, err := getUrl()
	if err != nil {
		logger.Error("Error getting upload URL", zap.Error(err))
		return Failed, err
	}

	logger.Info("Uploading file", zap.String("url", uploadUrl))
	var result ResultState
	if result, err = upload(uploadUrl, packageDir); err != nil {
		logger.Error("Error uploading file", zap.Error(err))
		return Failed, err
	}

	if result == Skipped {
		logger.Warn("File already exists", zap.String("packagePath", packageDir))
	} else {
		logger.Info("Successfully uploaded file", zap.String("packageDir", packageDir))
	}
	return Success, nil
}

// NewBaseProvider creates a new BaseProvider with common initialization logic
func NewBaseProvider(packageType, sourceHostname, targetHostname string, isContainer bool) BaseProvider {
	if sourceHostname == "" {
		sourceHostname = "github.com"
	}
	if targetHostname == "" {
		targetHostname = "github.com"
	}

	var sourceRegistryUrl, targetRegistryUrl string
	if isContainer {
		sourceRegistryUrl = "ghcr.io"
		targetRegistryUrl = "ghcr.io"
	} else {
		sourceRegistryUrl = fmt.Sprintf("https://%s.pkg.%s/", packageType, sourceHostname)
		targetRegistryUrl = fmt.Sprintf("https://%s.pkg.%s/", packageType, targetHostname)
	}

	return BaseProvider{
		PackageType:       packageType,
		SourceRegistryUrl: utils.ParseUrl(sourceRegistryUrl),
		TargetRegistryUrl: utils.ParseUrl(targetRegistryUrl),
		SourceHostnameUrl: utils.ParseUrl(fmt.Sprintf("https://%s/", sourceHostname)),
		TargetHostnameUrl: utils.ParseUrl(fmt.Sprintf("https://%s/", targetHostname)),
	}
}
