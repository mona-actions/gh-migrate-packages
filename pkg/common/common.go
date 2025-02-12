package common

import (
	"fmt"

	"github.com/mona-actions/gh-migrate-packages/internal/api"
	"github.com/mona-actions/gh-migrate-packages/internal/providers"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const ARE_YOU_SURE_YOU_EXPORTED = "Are you sure you exported first? gh migrate-packages export --help"

type Report struct {
	PackageSuccess     int
	VersionSuccess     int
	FileSuccess        int
	PackagesSkipped    int
	VersionsSkipped    int
	FilesSkipped       int
	PackagesFailed     int
	VersionsFailed     int
	FilesFailed        int
	PackagesByType     map[string]int
	currentPackageType string
}

func NewReport() *Report {
	return &Report{
		PackageSuccess:  0,
		VersionSuccess:  0,
		FileSuccess:     0,
		PackagesSkipped: 0,
		VersionsSkipped: 0,
		FilesSkipped:    0,
		PackagesFailed:  0,
		VersionsFailed:  0,
		FilesFailed:     0,
		PackagesByType:  make(map[string]int),
	}
}

func (r *Report) Print(name string) {
	pterm.Info.Printf("%s Report\n", name)
	pterm.Info.Println("Total Packages:", r.PackageSuccess+r.PackagesSkipped+r.PackagesFailed)
	pterm.Info.Println("Total Versions:", r.VersionSuccess+r.VersionsSkipped+r.VersionsFailed)
	pterm.Info.Println("Total Files:", r.FileSuccess+r.FilesSkipped+r.FilesFailed)
	pterm.Info.Println("Success Packages:", r.PackageSuccess)
	pterm.Info.Println("Success Versions:", r.VersionSuccess)
	pterm.Info.Println("Success Files:", r.FileSuccess)
	pterm.Info.Println("Skipped Packages:", r.PackagesSkipped)
	pterm.Info.Println("Skipped Versions:", r.VersionsSkipped)
	pterm.Info.Println("Skipped Files:", r.FilesSkipped)
	pterm.Info.Println("Failed Packages:", r.PackagesFailed)
	pterm.Info.Println("Failed Versions:", r.VersionsFailed)
	pterm.Info.Println("Failed Files:", r.FilesFailed)
}

func (r *Report) IncPackages(result providers.ResultState) {
	switch result {
	case providers.Success:
		r.PackageSuccess++
		if packageType := r.currentPackageType; packageType != "" {
			r.PackagesByType[packageType]++
		}
	case providers.Skipped:
		r.PackagesSkipped++
	case providers.Failed:
		r.PackagesFailed++
	}
}

func (r *Report) IncVersions(result providers.ResultState) {
	switch result {
	case providers.Success:
		r.VersionSuccess++
	case providers.Skipped:
		r.VersionsSkipped++
	case providers.Failed:
		r.VersionsFailed++
	}
}

func (r *Report) IncFiles(result providers.ResultState) {
	switch result {
	case providers.Success:
		r.FileSuccess++
	case providers.Skipped:
		r.FilesSkipped++
	case providers.Failed:
		r.FilesFailed++
	}
}

type ProcessCallback func(
	logger *zap.Logger,
	provider providers.Provider,
	report *Report,
	repository,
	packageType,
	packageName,
	version string,
	filenames []string) error

func ProcessPackages(logger *zap.Logger, packages [][]string, fn ProcessCallback, skipIfExists bool) (*Report, error) {

	report := NewReport()
	desiredPackageType := viper.GetString("PACKAGE_TYPE")
	var provider providers.Provider
	var err error

	pkgs := utils.GetListOfUniqueEntries(packages, []int{0, 1, 2, 3})
	for i, pkg := range pkgs {
		if i == 0 {
			continue
		}
		owner := pkg[0]
		repository := pkg[1]
		packageType := pkg[2]
		packageName := pkg[3]

		if desiredPackageType != "" && packageType != desiredPackageType {
			continue
		}

		if provider == nil || provider.GetPackageType() != packageType {
			logger.Info("Creating provider", zap.String("packageType", packageType))
			provider, err = providers.NewProvider(logger, packageType)
			if err != nil {
				logger.Error("Error creating provider", zap.Error(err))
				return report, err
			}

			if provider == nil {
				logger.Error("Provider is nil")
				return report, fmt.Errorf("provider is nil")
			}

			if err = provider.Connect(logger); err != nil {
				logger.Error("Error connecting to provider", zap.Error(err))
				return report, err
			}

		}

		// Only check on upload
		if skipIfExists {
			exists, err := api.PackageExists(packageName, packageType)
			if err != nil {
				report.IncPackages(providers.Failed)
				return report, err
			}

			if exists {
				report.IncPackages(providers.Skipped)
				logger.Info("Package already exists, skipping...", zap.String("package", packageName))
				continue
			}
		}

		report.currentPackageType = packageType

		versionFilters := map[string]string{
			"0": owner,       // org
			"1": repository,  // repo
			"2": packageType, // package name
			"3": packageName,
		}
		versions := utils.GetFlatListOfColumn(packages, versionFilters, 4)

		versionsSkipped := report.VersionsSkipped
		versionsFailed := report.VersionsFailed
		for i := len(versions) - 1; i >= 0; i-- {
			version := versions[i]
			fileFilters := map[string]string{
				"0": owner,       // org
				"1": repository,  // repo
				"2": packageType, // package name
				"3": packageName, // version
				"4": version,
			}
			filenames := utils.GetFlatListOfColumn(packages, fileFilters, 5)
			filesSkipped := report.FilesSkipped
			filesFailed := report.FilesFailed
			err := fn(logger, provider, report, repository, packageType, packageName, version, filenames)
			if report.FilesFailed > filesFailed {
				report.IncVersions(providers.Failed)
			} else if report.FilesSkipped > filesSkipped {
				report.IncVersions(providers.Skipped)
			} else {
				report.IncVersions(providers.Success)
			}
			if err != nil {
				return report, err
			}
		}
		if report.VersionsFailed > versionsFailed {
			report.IncPackages(providers.Failed)
		} else if report.VersionsSkipped > versionsSkipped {
			report.IncPackages(providers.Skipped)
		} else {
			report.IncPackages(providers.Success)
		}
	}

	return report, nil
}

func (r *Report) GetPackages(state providers.ResultState) int {
	switch state {
	case providers.Success:
		return r.PackageSuccess
	case providers.Skipped:
		return r.PackagesSkipped
	case providers.Failed:
		return r.PackagesFailed
	default:
		return 0
	}
}

func (r *Report) GetPackage(state providers.ResultState) int {
	switch state {
	case providers.Success:
		return r.PackageSuccess
	case providers.Skipped:
		return r.PackagesSkipped
	case providers.Failed:
		return r.PackagesFailed
	default:
		return 0
	}
}

func (r *Report) GetTotalSuccess() int {
	return r.PackageSuccess
}

func (r *Report) GetSuccessByType(packageType string) int {
	return r.PackagesByType[packageType]
}

func (r *Report) GetTotalFailures() int {
	return r.PackagesFailed
}
