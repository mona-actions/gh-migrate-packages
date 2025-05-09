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
			"GHMPKG_TARGET_HOSTNAME":     true,
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
	syncCmd.Flags().StringP("source-organization", "o", "", "Organization (required)")
	syncCmd.Flags().StringP("target-organization", "p", "", "Organization (required)")
	syncCmd.Flags().StringP("target-token", "t", "", "GitHub token (required)")

	viper.BindPFlag("GHMPKG_TARGET_HOSTNAME", syncCmd.Flags().Lookup("target-hostname"))
	viper.BindPFlag("GHMPKG_SOURCE_ORGANIZATION", syncCmd.Flags().Lookup("source-organization"))
	viper.BindPFlag("GHMPKG_TARGET_ORGANIZATION", syncCmd.Flags().Lookup("target-organization"))
	viper.BindPFlag("GHMPKG_TARGET_TOKEN", syncCmd.Flags().Lookup("target-token"))
}
