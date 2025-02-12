package sync

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/mona-actions/gh-migrate-packages/internal/files"
	"github.com/mona-actions/gh-migrate-packages/internal/providers"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/mona-actions/gh-migrate-packages/pkg/common"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)


func checkPath(logger *zap.Logger) {
	packageType := viper.GetString("GHMPKG_PACKAGE_TYPE")
	switch packageType {
	case "nuget":
		if !utils.FileExists("./tool/gpr") {
			utils.EnsureDirExists("./tool")
			installCmd := exec.Command("dotnet", "tool", "install", "gpr", "--add-source", "https://api.nuget.org/v3/index.json", "--tool-path", "./tool")
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
			if err := installCmd.Run(); err != nil {
				fmt.Println("Error installing gpr tool")
				logger.Error("Error installing gpr tool", zap.Error(err))
				panic(err)
			}
		}
		break
	default:
		break
	}
}

func Upload(logger *zap.Logger, provider providers.Provider, report *common.Report, repository, packageType, packageName, version string, filenames []string) error {
	owner := viper.GetString("GHMPKG_TARGET_ORGANIZATION")
	zapFields := []zap.Field{
		zap.String("owner", owner),
		zap.String("repository", repository),
		zap.String("packageType", packageType),
		zap.String("packageName", packageName),
		zap.String("version", version),
	}
	var err error
	for _, filename := range filenames {
		logger.Info("Uploading package", append(zapFields, zap.String("filename", filename))...)
		result, err := provider.Upload(logger, owner, repository, packageType, packageName, version, filename)
		report.IncFiles(result)
		if err != nil {
			logger.Error("Failed to upload package", append(zapFields, zap.String("filename", filename), zap.Error(err))...)
			break
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func Sync(logger *zap.Logger) error {
	utils.ResetRequestCounters()
	// Get all releases from source repository
	checkPath(logger)
	owner := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	desiredPackageType := viper.GetString("GHMPKG_PACKAGE_TYPE")
	prefix := owner
	if desiredPackageType != "" {
		prefix = fmt.Sprintf("%s-%s", owner, desiredPackageType)
	}
	exportFilename := fmt.Sprintf("./export/%s-packages.csv", prefix)
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Uploading packages from source org: %s", owner))

	// check if exportFilename exists
	if !utils.FileExists(exportFilename) {
		spinner.Fail(common.ARE_YOU_SURE_YOU_EXPORTED)
		return fmt.Errorf(common.ARE_YOU_SURE_YOU_EXPORTED)
	}

	packages, err := files.ReadCSV(exportFilename)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Error reading CSV file: %v", err))
		return err
	}

	var report *common.Report
	if report, err = common.ProcessPackages(logger, packages, Upload, true); err != nil {
		spinner.Fail(fmt.Sprintf("Error syncing package: %v", err))
		return err
	}
	spinner.Success()
	report.Print("Sync")
	return nil
}
