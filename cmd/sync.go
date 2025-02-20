// package cmd

// import (
// 	"github.com/mona-actions/gh-migrate-packages/pkg/sync"
// 	"github.com/spf13/cobra"
// 	"go.uber.org/zap"
// )

// // syncCmd represents the export command
// var syncCmd = &cobra.Command{
// 	Use:   "sync",
// 	Short: "Uploads the source organization's packages",
// 	Long:  "Uploads the source organization's packages",
// 	Run: func(cmd *cobra.Command, args []string) {
// 		logger := zap.L()
// 		if err := sync.Sync(logger); err != nil {
// 			logger.Error("Error syncing packages", zap.Error(err))
// 			panic(err)
// 		}
// 	},
// }

// func init() {
// 	rootCmd.AddCommand(syncCmd)
// }

package cmd

import (
	"fmt"

	"github.com/mona-actions/gh-migrate-packages/pkg/sync"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "syncs packages to the target organization",
	Long:  "syncs packages to the target organization",
	Run: func(cmd *cobra.Command, args []string) {
		GetFlagOrEnv(cmd, map[string]bool{
			"GHMPKG_TARGET_HOSTNAME":     false,
			"GHMPKG_TARGET_ORGANIZATION": true,
			"GHMPKG_TARGET_TOKEN":        true,
		})

		logger := zap.L()
		ShowConnectionStatus("sync")
		if err := sync.Sync(logger); err != nil {
			fmt.Printf("failed to sync packages: %v\n", err)
		}
	},
}

func init() {
	syncCmd.Flags().StringP("target-hostname", "n", "", "GitHub Enterprise Server hostname URL (optional)")
	syncCmd.Flags().StringP("target-organization", "o", "", "Organization (required)")
	syncCmd.Flags().StringP("target-token", "t", "", "GitHub token (required)")

	viper.BindPFlag("GHMPKG_TARGET_HOSTNAME", syncCmd.Flags().Lookup("target-hostname"))
	viper.BindPFlag("GHMPKG_TARGET_ORGANIZATION", syncCmd.Flags().Lookup("target-organization"))
	viper.BindPFlag("GHMPKG_TARGET_TOKEN", syncCmd.Flags().Lookup("target-token"))
}
