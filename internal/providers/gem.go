package providers

import (
	"fmt"
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

type RubyGemsProvider struct {
	BaseProvider
}

func NewRubyGemsProvider(logger *zap.Logger, packageType string) Provider {
	return &RubyGemsProvider{
		BaseProvider: NewBaseProvider(packageType, "", "", false),
	}
}

func (p *RubyGemsProvider) Connect(logger *zap.Logger) error {
	return nil
}

func (p *RubyGemsProvider) FetchPackageFiles(logger *zap.Logger, owner, repository, packageType, packageName, version string, metadata *github.PackageMetadata) ([]string, ResultState, error) {
	filenames := []string{
		fmt.Sprintf("%s-%s.gem", packageName, version),
	}
	return filenames, Success, nil
}

func (p *RubyGemsProvider) Export(logger *zap.Logger, owner string, content interface{}) error {
	return p.BaseProvider.Export(logger, owner, content)
}

func (p *RubyGemsProvider) Download(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
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

func (p *RubyGemsProvider) Rename(logger *zap.Logger, repository, filename string) error {
	// Replace the organization name in the content
	sourceHostname := utils.ParseUrl(viper.GetString("GHMPKG_SOURCE_HOSTNAME"))
	targetHostname := utils.ParseUrl(viper.GetString("GHMPKG_TARGET_HOSTNAME"))
	sourceHostname.Path = path.Join(sourceHostname.Path, viper.GetString("GHMPKG_SOURCE_ORGANIZATION"))
	targetHostname.Path = path.Join(targetHostname.Path, viper.GetString("GHMPKG_TARGET_ORGANIZATION"))
	if err := utils.RenameFileOccurances(filename, sourceHostname.String(), targetHostname.String(), -1); err != nil {
		return err
	}
	if err := utils.RenameFileOccurances(filename, p.SourceRegistryUrl.String(), p.TargetRegistryUrl.String(), -1); err != nil {
		return err
	}
	return nil
}

func (p *RubyGemsProvider) push(owner, dir, gemFile string) error {
	// Run gem publish
	pushUrl := *p.TargetRegistryUrl
	pushUrl.Path = path.Join(pushUrl.Path, owner)
	pushCmd := exec.Command("gem", "push", "--key", "github", "--host", pushUrl.String(), gemFile)
	pushCmd.Dir = dir
	pushCmd.Env = append(os.Environ(), "HTTPS_PROXY=", "GITHUB_TOKEN="+viper.GetString("GHMPKG_TARGET_TOKEN"))

	// Capture output to gemlog file
	pushLogFile, err := os.Create(filepath.Join(pushCmd.Dir, "gempush.log"))
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer pushLogFile.Close()

	pushCmd.Stdout = pushLogFile
	pushCmd.Stderr = pushLogFile

	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("failed to publish package: %w", err)
	}
	return nil
}

func (p *RubyGemsProvider) Upload(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
	return p.uploadPackage(
		logger, owner, repository, packageType, packageName, version, filename,
		func() (string, error) {
			return p.GetUploadUrl(logger, owner, repository, packageName, version, filename)
		},
		func(uploadUrl, packageDir string) (ResultState, error) {
			// Extract the tgz file
			cmd := exec.Command("gem", "unpack", filename)
			cmd.Dir = packageDir
			if err := cmd.Run(); err != nil {
				return Failed, fmt.Errorf("failed to extract package: %w", err)
			}

			gemBasename := strings.TrimSuffix(filename, ".gem")
			gemUnpackedDir := filepath.Join(packageDir, gemBasename)
			possibleGemFiles := []string{
				gemBasename,
				packageName,
			}
			for _, possibleGemFile := range possibleGemFiles {
				gemspecFile := filepath.Join(gemUnpackedDir, fmt.Sprintf("%s.gemspec", possibleGemFile))

				if !utils.FileExists(gemspecFile) {
					logger.Warn("Gemspec file not found", zap.String("gemFile", gemspecFile))
					continue
				}

				if err := p.Rename(logger, repository, gemspecFile); err != nil {
					return Failed, fmt.Errorf("failed to rename gemspec: %w", err)
				}

				// Run gem publish
				buildCmd := exec.Command("gem", "build", gemspecFile)
				buildCmd.Dir = gemUnpackedDir

				// Capture output to gemlog file
				buildLogFile, err := os.Create(filepath.Join(packageDir, "gembuild.log"))
				if err != nil {
					return Failed, fmt.Errorf("failed to create log file: %w", err)
				}
				defer buildLogFile.Close()

				buildCmd.Stdout = buildLogFile
				buildCmd.Stderr = buildLogFile

				if err := buildCmd.Run(); err != nil {
					return Failed, fmt.Errorf("failed to build package: %w", err)
				}

				if err = p.push(owner, gemUnpackedDir, fmt.Sprintf("%s-%s.gem", packageName, version)); err != nil {
					logger.Error("Failed to push package", zap.Error(err))
					return Failed, err
				}

				return Success, nil
			}

			logger.Warn("Gemspec file not found, pushing what was downloaded", zap.String("possibleGemFiles", fmt.Sprintf("%v", possibleGemFiles)))
			if err := p.push(owner, packageDir, filename); err != nil {
				logger.Error("Failed to push package", zap.Error(err))
				return Failed, err
			}

			return Success, nil
		},
	)
}

func (p *RubyGemsProvider) GetDownloadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error) {
	downloadUrl := *p.SourceRegistryUrl
	downloadUrl.Path = path.Join(downloadUrl.Path, owner, "gems", filename)
	return downloadUrl.String(), nil
}

func (p *RubyGemsProvider) GetUploadUrl(logger *zap.Logger, owner, repository, packageName, version string, filename string) (string, error) {
	uploadUrl := *p.TargetRegistryUrl
	uploadUrl.Path = path.Join(uploadUrl.Path, owner, repository, packageName, version, filename)
	return uploadUrl.String(), nil
}
