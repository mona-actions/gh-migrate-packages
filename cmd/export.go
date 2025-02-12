// package cmd

// import (
// 	"github.com/mona-actions/gh-migrate-packages/pkg/export"
// 	"github.com/spf13/cobra"
// 	"go.uber.org/zap"
// )

// // exportCmd represents the export command
// var exportCmd = &cobra.Command{
// 	Use:   "export",
// 	Short: "Exports the source organization's packages to CSV",
// 	Long:  "Exports the source organization's packages to CSV",
// 	Run: func(cmd *cobra.Command, args []string) {
// 		logger := zap.L()
// 		if err := export.Export(logger); err != nil {
// 			logger.Error("Error exporting packages", zap.Error(err))
// 			panic(err)
// 		}
// 	},
// }

// func init() {
// 	rootCmd.AddCommand(exportCmd)
// }

package cmd

import (
	"fmt"

	"github.com/mona-actions/gh-migrate-packages/pkg/export"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Exports a list of package data to a CSV file",
	Long:  "Exports a list of package data to a CSV file",
	Run: func(cmd *cobra.Command, args []string) {
		GetFlagOrEnv(cmd, map[string]bool{
			"GHMPKG_SOURCE_HOSTNAME":     false,
			"GHMPKG_SOURCE_ORGANIZATION": true,
			"GHMPKG_SOURCE_TOKEN":        true,
			"GHMPKG_PACKAGE_TYPE":        false,
		})

		logger := zap.L()
		ShowConnectionStatus("export")
		if err := export.Export(logger); err != nil {
			fmt.Printf("failed to export packages: %v\n", err)
		}
	},
}

func init() {
	exportCmd.Flags().StringP("source-hostname", "n", "", "GitHub Enterprise Server hostname URL (optional)")
	exportCmd.Flags().StringP("source-organization", "o", "", "Organization (required)")
	exportCmd.Flags().StringP("source-token", "t", "", "GitHub token (required)")
	exportCmd.Flags().StringSliceP("package-types", "p", []string{}, "Package type(s) to process (can be specified multiple times)")

	viper.BindPFlag("GHMPKG_SOURCE_HOSTNAME", exportCmd.Flags().Lookup("source-hostname"))
	viper.BindPFlag("GHMPKG_SOURCE_ORGANIZATION", exportCmd.Flags().Lookup("source-organization"))
	viper.BindPFlag("GHMPKG_SOURCE_TOKEN", exportCmd.Flags().Lookup("source-token"))
	viper.BindPFlag("GHMPKG_PACKAGE_TYPES", exportCmd.Flags().Lookup("package-types"))
}
