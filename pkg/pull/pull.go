package pull

import (
	"fmt"
	"os"
	"strings"
	"sync"
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

func Download(logger *zap.Logger, provider providers.Provider, report *common.Report, repository, packageType, packageName, version string, filenames []string) error {
	owner := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	zapFields := []zap.Field{
		zap.String("owner", owner),
		zap.String("repository", repository),
		zap.String("packageType", packageType),
		zap.String("packageName", packageName),
		zap.String("version", version),
	}

	logger.Info("Download function entry",
		zap.String("packageType", packageType),
		zap.String("packageName", packageName),
		zap.String("version", version),
		zap.Strings("filenames", filenames),
		zap.String("owner", owner),
		zap.String("repository", repository))

	// Add provider type logging
	logger.Info("Using provider",
		zap.String("packageType", packageType),
		zap.String("providerType", fmt.Sprintf("%T", provider)))

	pterm.Info.Println(fmt.Sprintf("📦 package: %s", packageName))
	pterm.Info.Println(fmt.Sprintf("🗃️ version: %s", version))

	// Create error channel to collect errors from workers
	errChan := make(chan error, len(filenames))

	// Create semaphore channel for concurrency control
	sem := make(chan struct{}, 5)

	// Create wait group to track when all downloads are complete
	var wg sync.WaitGroup

	// Launch workers for each filename
	for _, filename := range filenames {
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() {
				// Release semaphore
				<-sem
			}()

			logger.Info("Starting download for file",
				zap.String("packageType", packageType),
				zap.String("packageName", packageName),
				zap.String("version", version),
				zap.String("filename", filename))

			if packageType == "container" {
				// Extract semantic version from filename for containers
				semanticVersion := strings.Split(filename, ":")[1]
				logger.Info("Processing container package",
					zap.String("packageName", packageName),
					zap.String("version", version),
					zap.String("semanticVersion", semanticVersion),
					zap.String("filename", filename),
					zap.String("owner", owner),
					zap.String("repository", repository))

				if result, err := provider.Download(logger, owner, repository, packageType, packageName, semanticVersion, filename); err != nil {
					logger.Error("Failed to download package", append(zapFields,
						zap.String("filename", filename),
						zap.String("semanticVersion", semanticVersion),
						zap.Error(err))...)
					pterm.Error.Println(fmt.Sprintf("    ❌ Failed to download: %s", filename))
					errChan <- fmt.Errorf("failed to download %s: %w", filename, err)
				} else {
					logger.Info("Download result",
						zap.String("packageName", packageName),
						zap.String("version", semanticVersion),
						zap.String("filename", filename),
						zap.Any("result", result))
					report.IncFiles(result)
					if result == providers.Success {
						pterm.Success.Println(fmt.Sprintf("✅ %s", filename))
					}
				}
			} else {
				logger.Info("Attempting non-container download",
					zap.String("packageType", packageType),
					zap.String("packageName", packageName),
					zap.String("version", version),
					zap.String("filename", filename))

				result, err := provider.Download(logger, owner, repository, packageType, packageName, version, filename)
				if err != nil {
					logger.Error("Failed to download package", append(zapFields,
						zap.String("filename", filename),
						zap.Error(err))...)
					pterm.Error.Println(fmt.Sprintf("❌ Failed to download: %s", filename))
					errChan <- fmt.Errorf("failed to download %s: %w", filename, err)
				} else {
					logger.Info("Download completed",
						zap.String("packageName", packageName),
						zap.String("version", version),
						zap.String("filename", filename),
						zap.Any("result", result))
					report.IncFiles(result)
					if result == providers.Success {
						pterm.Success.Println(fmt.Sprintf("✅ %s", filename))
					}
				}
			}
		}(filename)
	}

	// Wait for all downloads to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	var errs []string
	for err := range errChan {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("download errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

func Pull(logger *zap.Logger) error {
	startTime := time.Now()
	owner := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	desiredPackageType := viper.GetString("GHMPKG_PACKAGE_TYPES")

	logger.Info("Starting pull process",
		zap.String("owner", owner),
		zap.String("desiredPackageType", desiredPackageType))

	pterm.Info.Println("Starting pull process...")
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Pulling packages from source org: %s", owner))

	// Add directory existence check
	if _, err := os.Stat("./migration-packages"); os.IsNotExist(err) {
		spinner.Fail("migration-packages directory not found")
		return fmt.Errorf("migration-packages directory not found: %w", err)
	}

	// Handle either specific package type or all package types
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

		// Check if package type directory exists
		pkgTypeDir := fmt.Sprintf("./migration-packages/export/%s", pkgType)
		if _, err := os.Stat(pkgTypeDir); os.IsNotExist(err) {
			logger.Warn("Package type directory not found",
				zap.String("packageType", pkgType),
				zap.String("directory", pkgTypeDir))
			continue
		}

		// Look for the most recent CSV file in the package type directory
		pattern := fmt.Sprintf("./migration-packages/export/%s/*_%s_%s_packages.csv", pkgType, owner, pkgType)
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
			spinner.Fail(fmt.Sprintf("Error reading CSV file for %s: %v", pkgType, err))
			return err
		}

		// Log the content of the first few rows to verify data
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

	// Debug logging before processing
	logger.Info("Final package list before processing",
		zap.Int("totalPackages", len(allPackages)))

	for i, pkg := range allPackages {
		logger.Info("Package in final list",
			zap.Int("index", i),
			zap.String("org", pkg[0]),
			zap.String("repo", pkg[1]),
			zap.String("type", pkg[2]),
			zap.String("name", pkg[3]),
			zap.String("version", pkg[4]),
			zap.String("filename", pkg[5]))
	}

	if len(allPackages) == 0 {
		spinner.Fail("No package export files found")
		return fmt.Errorf("no package export files found")
	}

	report, err := common.ProcessPackages(logger, allPackages, Download, false)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Error pulling package: %v", err))
		return err
	}

	spinner.Success("Pull completed")

	// Calculate duration
	duration := time.Since(startTime)
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	// Print detailed summary
	fmt.Println("\n📊 Pull Summary:")
	fmt.Printf("✅ Successfully processed: %d packages\n", report.PackageSuccess)
	fmt.Printf("❌ Failed: %d packages\n", report.PackagesFailed)

	for _, pkgType := range SUPPORTED_PACKAGE_TYPES {
		if count := len(packageStats[pkgType]); count > 0 {
			emoji := "📦"
			name := pkgType
			fmt.Printf("  %s %s: %d\n", emoji, name, count)
		}
	}

	fmt.Println("📁 Output directory: migration-packages/packages")
	fmt.Printf("🕐 Total time: %dh %dm %ds\n\n", hours, minutes, seconds)
	fmt.Println("✅ Pull completed successfully!")

	return nil
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
