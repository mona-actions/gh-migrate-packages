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

	pterm.Info.Println("Starting export to csv...")
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Exporting packages from source org: %s", owner))

	// Create base export directory
	baseDir := "./migration-packages/export"
	if err := files.EnsureDir(baseDir); err != nil {
		spinner.Fail(fmt.Sprintf("Error creating base directory: %v", err))
		return err
	}

	// Validate and filter package types
	packageTypes := make([]string, 0)
	if len(desiredPackageTypes) > 0 {
		pterm.Info.Println(fmt.Sprintf("🔍 Filtering for package types: %v", desiredPackageTypes))
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
				spinner.Fail(fmt.Sprintf("❌ Unsupported package type: %s", desired))
				return fmt.Errorf("unsupported package type: %s", desired)
			}
		}
	} else {
		packageTypes = SUPPORTED_PACKAGE_TYPES
		pterm.Info.Println("📦 Exporting all supported package types")
	}

	for _, packageType := range packageTypes {
		pterm.Info.Println(fmt.Sprintf("📦 Processing %s packages...", packageType))

		// Initialize CSV data for this package type
		packagesCSV := [][]string{
			{"organization", "repository", "package_type", "package_name", "package_version", "package_filename"},
		}

		provider, err := providers.NewProvider(logger, packageType)
		if err != nil {
			spinner.Fail(fmt.Sprintf("❌ Error creating provider: %v", err))
			return err
		}

		packages, err := api.FetchPackages(packageType)
		if err != nil {
			spinner.Fail(fmt.Sprintf("❌ Error getting packages: %v", err))
			return err
		}

		packageStats[packageType] = len(packages)
		totalPackages += len(packages)
		pterm.Info.Println(fmt.Sprintf("📊 Found %d %s packages", len(packages), packageType))

		// Process packages and add to packagesCSV
		for i, pkg := range packages {
			reposWithPackages[pkg.Repository.GetName()] = true
			pterm.Info.Printf("  package %d/%d: %s\n", i+1, len(packages), pkg.GetName())

			versions, err := api.FetchPackageVersions(pkg)
			if err != nil {
				spinner.Fail(fmt.Sprintf("❌ Error getting versions: %v", err))
				return err
			}
			pterm.Info.Printf("    Found %d versions\n", len(versions))

			for _, version := range versions {
				filenames, result, err := provider.FetchPackageFiles(logger, owner, pkg.Repository.GetName(), packageType, pkg.GetName(), version.GetName(), version.Metadata)
				if result != providers.Success {
					report.IncPackages(result)
					report.IncVersions(result)
					pterm.Warning.Printf("    ⚠️  Version %s: %s\n", version.GetName(), result)
				}
				if err != nil {
					spinner.Fail(fmt.Sprintf("❌ Error fetching package files: %v", err))
					return err
				}

				for _, filename := range filenames {
					report.IncFiles(result)
					packagesCSV = append(packagesCSV, []string{owner, pkg.Repository.GetName(), packageType, pkg.GetName(), version.GetName(), filename})
					if result == providers.Success {
						pterm.Success.Printf(" ✅ %s", filename)
					}
				}
				report.IncVersions(providers.Success)
			}
			report.IncPackages(providers.Success)
		}

		// Create package type directory
		packageDir := filepath.Join(baseDir, packageType)
		if err := files.EnsureDir(packageDir); err != nil {
			spinner.Fail(fmt.Sprintf("❌ Error creating package directory: %v", err))
			return err
		}

		// Create CSV file for this package type
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		csvName := fmt.Sprintf("%s_%s_%s_packages.csv", timestamp, owner, packageType)
		filename := filepath.Join(packageDir, csvName)
		if err := files.CreateCSV(packagesCSV, filename); err != nil {
			spinner.Fail(fmt.Sprintf("❌ Error creating CSV: %v", err))
			return err
		}
		pterm.Success.Printf("✅ Created CSV file: %s", csvName)
		fmt.Println()
	}

	spinner.Success("Packages exported successfully")

	// Calculate duration
	duration := time.Since(startTime)
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	// Print detailed report
	fmt.Println("\n📊 Export Summary:")
	fmt.Printf("Total packages found: %d\n", totalPackages)
	fmt.Printf("✅ Successfully processed: %d packages\n", report.GetPackages(providers.Success))

	// Print package type breakdown
	for _, pkgType := range SUPPORTED_PACKAGE_TYPES {
		if count, exists := packageStats[pkgType]; exists && count > 0 {
			emoji := "📦"
			name := pkgType
			fmt.Printf("  %s %s: %d\n", emoji, name, count)
		}
	}

	fmt.Printf("❌ Failed to process: %d packages\n", report.GetPackages(providers.Failed))
	fmt.Printf("🔍 Repositories with packages: %d\n", len(reposWithPackages))
	fmt.Printf("📁 Output directory: %s\n", baseDir)
	fmt.Printf("🕐 Total time: %dh %dm %ds\n\n", hours, minutes, seconds)
	fmt.Println("✅ Export completed successfully!")

	return nil
}
