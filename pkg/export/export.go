package export

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/mona-actions/gh-migrate-packages/internal/api"
	"github.com/mona-actions/gh-migrate-packages/internal/files"
	"github.com/mona-actions/gh-migrate-packages/internal/providers"
	"github.com/mona-actions/gh-migrate-packages/pkg/common"

	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// var SUPPORTED_PACKAGE_TYPES = []string{"maven", "npm", "container", "rubygems", "nuget"}
var SUPPORTED_PACKAGE_TYPES = []string{"maven", "npm", "container", "rubygems", "nuget"}

func Export(logger *zap.Logger) error {
	startTime := time.Now()
	report := common.NewReport()
	packageStats := make(map[string]int)
	totalPackages := 0
	reposWithPackages := make(map[string]bool)
	owner := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	desiredPackageTypes := viper.GetStringSlice("GHMPKG_PACKAGE_TYPES")
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Exporting packages from source org: %s", owner))

	// Create base export directory
	baseDir := "./migration-packages/export"
	if err := files.EnsureDir(baseDir); err != nil {
		spinner.Fail(fmt.Sprintf("Error creating base directory: %v", err))
		return err
	}

	// Get current timestamp for filename
	timestamp := time.Now().Format("2006-01-02_15-04-05")

	// Validate and filter package types
	packageTypes := make([]string, 0)
	if len(desiredPackageTypes) > 0 {
		// Validate each desired package type against supported types
		for _, desired := range desiredPackageTypes {
			isSupported := false
			for _, supported := range SUPPORTED_PACKAGE_TYPES {
				if desired == supported {
					isSupported = true
					packageTypes = append(packageTypes, desired)
					break
				}
			}
			if !isSupported {
				spinner.Fail(fmt.Sprintf("Unsupported package type: %s", desired))
				return fmt.Errorf("unsupported package type: %s", desired)
			}
		}
	} else {
		packageTypes = SUPPORTED_PACKAGE_TYPES // Use all supported types if none specified
	}

	for _, packageType := range packageTypes {
		// Initialize CSV data for this package type
		packagesCSV := [][]string{
			{"organization", "repository", "package_type", "package_name", "package_version", "package_filename"},
		}

		provider, err := providers.NewProvider(logger, packageType)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Error creating provider: %v", err))
			return err
		}

		// ... existing package fetching code ...
		packages, err := api.FetchPackages(packageType)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Error getting packages: %v", err))
			return err
		}

		packageStats[packageType] = len(packages)
		totalPackages += len(packages)

		// Process packages and add to packagesCSV
		for _, pkg := range packages {
			reposWithPackages[pkg.Repository.GetName()] = true
			// ... existing version processing code ...
			versions, err := api.FetchPackageVersions(pkg)
			if err != nil {
				spinner.Fail(fmt.Sprintf("Error getting versions: %v", err))
				return err
			}
			for _, version := range versions {
				filenames, result, err := provider.FetchPackageFiles(logger, owner, pkg.Repository.GetName(), packageType, pkg.GetName(), version.GetName(), version.Metadata)
				if result != providers.Success {
					report.IncPackages(result)
					report.IncVersions(result)
				}
				if err != nil {
					spinner.Fail(fmt.Sprintf("Error fetching package files: %v", err))
					return err
				}
				for _, filename := range filenames {
					report.IncFiles(result)
					packagesCSV = append(packagesCSV, []string{owner, pkg.Repository.GetName(), packageType, pkg.GetName(), version.GetName(), filename})
				}
				report.IncVersions(providers.Success)
			}
			report.IncPackages(providers.Success)
		}

		// Create package type directory
		packageDir := filepath.Join(baseDir, packageType)
		if err := files.EnsureDir(packageDir); err != nil {
			spinner.Fail(fmt.Sprintf("Error creating package directory: %v", err))
			return err
		}

		// Create CSV file for this package type
		filename := filepath.Join(packageDir, fmt.Sprintf("%s_%s_%s_packages.csv", timestamp, owner, packageType))
		if err := files.CreateCSV(packagesCSV, filename); err != nil {
			spinner.Fail(fmt.Sprintf("Error creating CSV: %v", err))
			return err
		}
	}

	spinner.Success("Packages exported successfully")

	// Calculate duration
	duration := time.Since(startTime)

	// Print detailed report
	fmt.Println("\n📊 Export Summary:")
	fmt.Printf("Total packages found: %d\n", totalPackages)
	fmt.Printf("✅ Successfully processed: %d packages\n", report.GetPackages(providers.Success))

	// Print package type breakdown
	for _, pkgType := range []string{"container", "rubygems", "maven", "npm", "nuget"} {
		if count, exists := packageStats[pkgType]; exists && count > 0 {
			emoji := "📦"
			name := pkgType
			if pkgType == "container" {
				name = "docker"
			}
			fmt.Printf("  %s %s: %d\n", emoji, name, count)
		}
	}

	fmt.Printf("❌ Failed to process: %d packages\n", report.GetPackages(providers.Failed))
	fmt.Printf("🔍 Repositories with packages: %d\n", len(reposWithPackages))
	fmt.Printf("📁 Output directory: %s\n", baseDir)
	fmt.Printf(" Total time: %ds\n\n", int(duration.Seconds()))
	fmt.Println("✅ Export completed successfully!")

	return nil
}
