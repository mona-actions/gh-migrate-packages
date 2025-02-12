package pull

import (
	"fmt"
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

var SUPPORTED_PACKAGE_TYPES = []string{"container", "rubygems", "maven", "npm", "nuget"}

func Download(logger *zap.Logger, provider providers.Provider, report *common.Report, repository, packageType, packageName, version string, filenames []string) error {
	owner := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	zapFields := []zap.Field{
		zap.String("owner", owner),
		zap.String("repository", repository),
		zap.String("packageType", packageType),
		zap.String("packageName", packageName),
		zap.String("version", version),
	}

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

			logger.Info("Downloading package", append(zapFields, zap.String("filename", filename))...)
			if result, err := provider.Download(logger, owner, repository, packageType, packageName, version, filename); err != nil {
				logger.Error("Failed to download package", append(zapFields, zap.String("filename", filename), zap.Error(err))...)
				errChan <- fmt.Errorf("failed to download %s: %w", filename, err)
			} else {
				report.IncFiles(result)
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
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Downloading packages from source org: %s", owner))

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
	for _, pkgType := range packageTypes {
		// Add debug logging for package type processing
		logger.Debug("Processing package type",
			zap.String("type", pkgType),
			zap.String("owner", owner))

		// Look for the most recent CSV file in the package type directory
		pattern := fmt.Sprintf("./migration-packages/%s/*_%s_%s_packages.csv", pkgType, owner, pkgType)
		logger.Debug("Searching for CSV with pattern",
			zap.String("pattern", pattern))

		matches, err := utils.FindMostRecentFile(pattern)
		if err != nil {
			// Try alternate pattern without owner in filename
			altPattern := fmt.Sprintf("./packages-migration/%s/*_%s_packages.csv", pkgType, pkgType)
			logger.Debug("Trying alternate pattern",
				zap.String("altPattern", altPattern))

			matches, err = utils.FindMostRecentFile(altPattern)
			if err != nil {
				logger.Warn("No export file found for package type",
					zap.String("packageType", pkgType),
					zap.String("pattern", pattern),
					zap.String("altPattern", altPattern),
					zap.Error(err))
				continue
			}
		}

		// Add logging for found CSV file
		logger.Info("Found CSV file",
			zap.String("packageType", pkgType),
			zap.String("file", matches))

		packages, err := files.ReadCSV(matches)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Error reading CSV file for %s: %v", pkgType, err))
			return err
		}

		// Add logging for package count
		logger.Info("Read packages from CSV",
			zap.String("packageType", pkgType),
			zap.Int("count", len(packages)))

		allPackages = append(allPackages, packages...)
	}

	// Add logging for total packages found
	logger.Info("Total packages to process",
		zap.Int("count", len(allPackages)))

	if len(allPackages) == 0 {
		spinner.Fail("No package export files found")
		return fmt.Errorf("no package export files found")
	}

	// Initialize report before processing packages
	report := common.NewReport()
	var err error

	if report, err = common.ProcessPackages(logger, allPackages, Download, false); err != nil {
		spinner.Fail(fmt.Sprintf("Error pulling package: %v", err))
		return err
	}

	spinner.Success("Packages downloaded successfully")

	// Add debug logging right before summary
	logger.Debug("Report status before summary",
		zap.Int("total_success", report.GetTotalSuccess()),
		zap.Int("total_failures", report.GetTotalFailures()))

	// Calculate elapsed time
	elapsed := time.Since(startTime)
	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60

	// Print detailed summary
	fmt.Println("\n📊 Summary:")
	fmt.Printf("✅ Successfully processed: %d packages\n", report.GetTotalSuccess())

	// Print package type counts
	for _, pkgType := range SUPPORTED_PACKAGE_TYPES {
		count := report.GetSuccessByType(pkgType)
		if count > 0 {
			fmt.Printf("  📦 %s: %d\n", pkgType, count)
		}
	}

	fmt.Printf("❌ Failed: %d packages\n", report.GetTotalFailures())
	fmt.Printf("📁 Output directory: package-migration/(%s)\n", strings.Join(packageTypes, ", "))
	fmt.Printf("🕐 Total time: %dh %dm %ds\n", hours, minutes, seconds)

	return nil
}
