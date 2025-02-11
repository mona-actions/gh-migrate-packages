package cmd

import (
	"fmt"

	"github.com/mona-actions/gh-migrate-packages/pkg/pull"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "pulls packages locally from the source organization",
	Long:  "pulls packages locally from the source organization",
	Run: func(cmd *cobra.Command, args []string) {
		GetFlagOrEnv(cmd, map[string]bool{
			"GHMPKG_SOURCE_HOSTNAME":     false,
			"GHMPKG_SOURCE_ORGANIZATION": true,
			"GHMPKG_SOURCE_TOKEN":        true,
		})

		logger := zap.L()
		ShowConnectionStatus("pull")
		if err := pull.Pull(logger); err != nil {
			fmt.Printf("failed to pull packages: %v\n", err)
		}
	},
}

func init() {
	pullCmd.Flags().StringP("source-hostname", "n", "", "GitHub Enterprise Server hostname URL (optional)")
	pullCmd.Flags().StringP("source-organization", "o", "", "Organization (required)")
	pullCmd.Flags().StringP("source-token", "t", "", "GitHub token (required)")

	viper.BindPFlag("GHMPKG_SOURCE_HOSTNAME", pullCmd.Flags().Lookup("source-hostname"))
	viper.BindPFlag("GHMPKG_SOURCE_ORGANIZATION", pullCmd.Flags().Lookup("source-organization"))
	viper.BindPFlag("GHMPKG_SOURCE_TOKEN", pullCmd.Flags().Lookup("source-token"))
}
