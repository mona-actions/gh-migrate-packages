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
	//syncCmd.Flags().StringP("target-hostname", "n", "", "GitHub Enterprise Server hostname URL (optional)")
	syncCmd.Flags().StringP("source-organization", "o", "", "Organization (required)")
	syncCmd.Flags().StringP("target-organization", "p", "", "Organization (required)")
	syncCmd.Flags().StringP("target-token", "t", "", "GitHub token (required)")
	syncCmd.Flags().StringP("migration-path", "m", "./migration-packages", "Path to the migration directory (default: ./migration-packages)")
	syncCmd.Flags().StringP("repository", "r", "", "Repository to sync (optional, syncs all repositories if not specified)")

	//viper.BindPFlag("GHMPKG_TARGET_HOSTNAME", syncCmd.Flags().Lookup("target-hostname"))
	viper.BindPFlag("GHMPKG_SOURCE_ORGANIZATION", syncCmd.Flags().Lookup("source-organization"))
	viper.BindPFlag("GHMPKG_TARGET_ORGANIZATION", syncCmd.Flags().Lookup("target-organization"))
	viper.BindPFlag("GHMPKG_TARGET_TOKEN", syncCmd.Flags().Lookup("target-token"))
	viper.BindPFlag("GHMPKG_MIGRATION_PATH", syncCmd.Flags().Lookup("migration-path"))
	viper.BindPFlag("GHMPKG_REPOSITORY", syncCmd.Flags().Lookup("repository"))
}
