package sync

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/mona-actions/gh-migrate-packages/internal/files"
	"github.com/mona-actions/gh-migrate-packages/internal/providers"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/mona-actions/gh-migrate-packages/pkg/common"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var SUPPORTED_PACKAGE_TYPES = common.SUPPORTED_PACKAGE_TYPES

func checkPath(logger *zap.Logger) {
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

	pterm.Info.Println(fmt.Sprintf("📦 package: %s", packageName))
	pterm.Info.Println(fmt.Sprintf("🗃️ version: %s", version))
	pterm.Info.Println(fmt.Sprintf("📦 type: %s", packageType))
	if repository != "" {
		pterm.Info.Println(fmt.Sprintf("📂 repository: %s", repository))
	} else {
		pterm.Info.Println("📂 repository: (n/a, org scoped)")
	}

	// Special case for Maven packages
	if mavenProvider, ok := provider.(*providers.MavenProvider); ok {
		results, err := mavenProvider.UploadBatch(logger, owner, repository, packageType, packageName, version, filenames)
		if err != nil {
			return err
		}
		for i, result := range results {
			report.IncFiles(result)
			if result == providers.Success {
				pterm.Success.Println(fmt.Sprintf("✅ %s", filenames[i]))
			}
		}
		return nil
	}

	// Regular sequential upload for other package types
	var err error
	for _, filename := range filenames {
		result, err := provider.Upload(logger, owner, repository, packageType, packageName, version, filename)
		if err != nil {
			logger.Error("Failed to upload package", append(zapFields,
				zap.String("filename", filename),
				zap.Error(err))...)
			pterm.Error.Println(fmt.Sprintf("❌ Failed to upload: %s", filename))
			return err
		}
		report.IncFiles(result)
		if result == providers.Success {
			pterm.Success.Println(fmt.Sprintf("✅ %s", filename))
		}
	}

	return err
}

func Sync(logger *zap.Logger) error {
	startTime := time.Now()
	utils.ResetRequestCounters()
	checkPath(logger)
	owner := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	targetOwner := viper.GetString("GHMPKG_TARGET_ORGANIZATION")
	desiredPackageType := viper.GetString("GHMPKG_PACKAGE_TYPE")

	pterm.Info.Println("Starting sync process...")
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Syncing packages to target org: %s", targetOwner))

	packageTypes := SUPPORTED_PACKAGE_TYPES
	if desiredPackageType != "" {
		if !utils.Contains(SUPPORTED_PACKAGE_TYPES, desiredPackageType) {
			spinner.Fail(fmt.Sprintf("Unsupported package type: %s", desiredPackageType))
			return fmt.Errorf("unsupported package type: %s", desiredPackageType)
		}
		packageTypes = []string{desiredPackageType}
	}

	var allPackages [][]string
	packageStats := make(map[string][]string)

	for _, pkgType := range packageTypes {
		logger.Info("Processing package type", zap.String("type", pkgType))
		pterm.Info.Println(fmt.Sprintf("Processing %s packages...", pkgType))
		pkgTypeDir := fmt.Sprintf("./migration-packages/export/%s", pkgType)
		if _, err := os.Stat(pkgTypeDir); os.IsNotExist(err) {
			logger.Warn("Package type directory not found",
				zap.String("packageType", pkgType),
				zap.String("directory", pkgTypeDir))
			continue
		}

		pattern := fmt.Sprintf("./migration-packages/export/%s/*_%s_%s_packages.csv", desiredPackageType, owner, desiredPackageType)
		logger.Info("Searching for CSV with pattern", zap.String("pattern", pattern))

		matches, err := utils.FindMostRecentFile(pattern)
		if err != nil {
			// Try alternate pattern without owner in filename
			altPattern := fmt.Sprintf("./migration-packages/export/%s/*_%s_packages.csv", pkgType, pkgType)
			logger.Info("Trying alternate pattern",
				zap.String("altPattern", altPattern))

			matches, err = utils.FindMostRecentFile(altPattern)
			if err != nil {
				logger.Warn("No export file found for package type",
					zap.String("packageType", pkgType),
					zap.Error(err))
				continue
			}
		}

		logger.Info("Found CSV file",
			zap.String("packageType", pkgType),
			zap.String("file", matches))

		packages, err := files.ReadCSV(matches)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Error reading CSV file: %v", err))
			return err
		}

		logger.Info("CSV content sample",
			zap.String("packageType", pkgType),
			zap.Int("totalRows", len(packages)),
			zap.Any("firstRows", packages[:min(len(packages), 3)]))

		if len(packages) <= 1 {
			pterm.Warning.Println(fmt.Sprintf("No package data found in CSV for %s", pkgType))
			continue
		}

		allPackages = append(allPackages, packages[1:]...)
		for _, pkg := range packages[1:] {
			if _, ok := packageStats[pkgType]; ok {
				if utils.Contains(packageStats[pkgType], pkg[3]) {
					continue
				} else {
					packageStats[pkgType] = append(packageStats[pkgType], pkg[3])
				}
			} else {
				packageStats[pkgType] = []string{pkg[3]}
			}
		}

		pterm.Info.Println(fmt.Sprintf("Found %d packages in CSV for %s", len(packageStats[pkgType]), pkgType))
	}

	var report *common.Report
	var err error
	if report, err = common.ProcessPackages(logger, allPackages, Upload, true); err != nil {
		spinner.Fail(fmt.Sprintf("Error syncing package: %v", err))
		return err
	}
	if report.PackageSuccess == 0 {
		spinner.Fail("No packages were synced")
	} else if report.PackagesFailed > 0 {
		spinner.Warning("Sync completed with some errors, Please check the logs for more details")
	} else {
		spinner.Success("Sync completed")
	}

	// Calculate duration
	duration := time.Since(startTime)
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	fmt.Println("\n📊 Sync Summary:")
	fmt.Printf("✅ Successfully processed: %d packages\n", report.PackageSuccess)
	fmt.Printf("❌ Failed: %d packages\n", report.PackagesFailed)

	for _, pkgType := range SUPPORTED_PACKAGE_TYPES {
		if count := len(packageStats[pkgType]); count > 0 {
			emoji := "📦"
			name := pkgType
			fmt.Printf("  %s %s: %d\n", emoji, name, count)
		}
	}

	//fmt.Printf("📁 Output directory: migration-packages/packages/(%s)\n", strings.Join(packageTypes, ", "))
	fmt.Printf("🕐 Total time: %dh %dm %ds\n\n", hours, minutes, seconds)
	fmt.Println("✅ Sync completed successfully!")

	return nil
}
