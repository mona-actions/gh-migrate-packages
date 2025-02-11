package export

import (
	"fmt"

	"github.com/mona-actions/gh-migrate-packages/internal/api"
	"github.com/mona-actions/gh-migrate-packages/internal/files"
	"github.com/mona-actions/gh-migrate-packages/internal/providers"
	"github.com/mona-actions/gh-migrate-packages/pkg/common"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

//var SUPPORTED_PACKAGE_TYPES = []string{"maven", "npm", "container", "rubygems", "nuget"}
var SUPPORTED_PACKAGE_TYPES = []string{"maven", "npm", "container", "rubygems", "nuget"}

func Export(logger *zap.Logger) error {
	// Get all teams from source organization
	report := common.NewReport()
	owner := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	println(owner)
	desiredPackageType := viper.GetString("PACKAGE_TYPE")
	prefix := owner
	if desiredPackageType != "" {
		prefix = fmt.Sprintf("%s-%s", owner, desiredPackageType)
	}
	filename := fmt.Sprintf("./export/%s-packages.csv", prefix)
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Exporting packages from source org: %s", owner))

	packagesCSV := [][]string{
		{"Organization", "Repository", "Package Type", "Package Name", "Package Version", "Package Filename"},
	}
	for _, packageType := range SUPPORTED_PACKAGE_TYPES {
		if desiredPackageType != "" && desiredPackageType != packageType {
			continue
		}
		provider, err := providers.NewProvider(logger, packageType)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Error creating provider: %v", err))
			return err
		}
		packages, err := api.FetchPackages(packageType)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Error getting packages: %v", err))
			return err
		}
		for _, pkg := range packages {
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
	}
	spinner.Success("Packages fetched successfully")
	spinner, _ = pterm.DefaultSpinner.Start("Creating CSV file...")

	err := files.CreateCSV(packagesCSV, filename)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Error creating CSV: %v", err))
		return err
	}
	spinner.Success(fmt.Sprintf("Packages saved successfully: %s", filename))
	report.Print("Export")
	return nil
}
