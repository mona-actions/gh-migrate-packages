package pull

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mona-actions/gh-migrate-packages/internal/files"
	"github.com/mona-actions/gh-migrate-packages/internal/providers"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/mona-actions/gh-migrate-packages/pkg/common"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var SUPPORTED_PACKAGE_TYPES = []string{"maven", "npm"}

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
	owner := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	desiredPackageType := viper.GetString("GHMPKG_PACKAGE_TYPE")
	prefix := owner
	if desiredPackageType != "" {
		prefix = fmt.Sprintf("%s-%s", owner, desiredPackageType)
	}
	exportFilename := fmt.Sprintf("./export/%s-packages.csv", prefix)
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Downloading packages from source org: %s", owner))
	println(exportFilename)

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
	if report, err = common.ProcessPackages(logger, packages, Download, false); err != nil {
		spinner.Fail(fmt.Sprintf("Error pulling package: %v", err))
		return err
	}

	spinner.Success(fmt.Sprintf("Packages downloaded successfully"))
	report.Print("Pull")
	return nil
}
