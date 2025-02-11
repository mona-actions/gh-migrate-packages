/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
// */
// package cmd

// import (
// 	"fmt"
// 	"os"
// 	"time"

// 	"github.com/spf13/cobra"
// 	"github.com/spf13/viper"
// 	"go.uber.org/zap"
// 	"go.uber.org/zap/zapcore"
// )

// // rootCmd represents the base command when called without any subcommands
// var rootCmd = &cobra.Command{
// 	Use:   "migrate-packages",
// 	Short: "gh cli extension to assist in the migration of packages between GitHub repositories",
// 	Long:  `gh cli extension to assist in the migration of packages between GitHub repositories`,
// 	// Uncomment the following line if your bare application
// 	// has an action associated with it:
// 	// Run: func(cmd *cobra.Command, args []string) { },
// }

// // Execute adds all child commands to the root command and sets flags appropriately.
// // This is called by main.main(). It only needs to happen once to the rootCmd.
// func Execute() {
// 	err := rootCmd.Execute()
// 	if err != nil {
// 		os.Exit(1)
// 	}
// }

// func init() {
// 	rootCmd.PersistentFlags().String("source-organization", "", "Organization of the repository")
// 	rootCmd.MarkFlagRequired("source-organization")

// 	rootCmd.PersistentFlags().String("target-organization", "", "Organization of the repository")
// 	rootCmd.MarkFlagRequired("target-organization")

// 	rootCmd.PersistentFlags().String("source-token", "", "GitHub token")
// 	rootCmd.MarkFlagRequired("source-token")

// 	rootCmd.PersistentFlags().String("target-token", "", "GitHub token")
// 	rootCmd.MarkFlagRequired("target-token")

// 	rootCmd.PersistentFlags().String("source-hostname", "github.com", "GitHub Enterprise hostname url (optional) Ex. github.example.com")
// 	rootCmd.PersistentFlags().String("target-hostname", "github.com", "GitHub Enterprise hostname url (optional) Ex. github.example.com")

// 	rootCmd.PersistentFlags().StringP("package-type", "p", "", "package type for which to filter")
// 	rootCmd.MarkFlagRequired("package-type")

// 	cobra.OnInitialize(initConfig, bindFlags)
// }

// func bindFlags() {
// 	viper.BindPFlag("SOURCE_ORGANIZATION", rootCmd.PersistentFlags().Lookup("source-organization"))
// 	viper.BindPFlag("TARGET_ORGANIZATION", rootCmd.PersistentFlags().Lookup("target-organization"))
// 	viper.BindPFlag("SOURCE_TOKEN", rootCmd.PersistentFlags().Lookup("source-token"))
// 	viper.BindPFlag("TARGET_TOKEN", rootCmd.PersistentFlags().Lookup("target-token"))
// 	viper.BindPFlag("SOURCE_HOSTNAME", rootCmd.PersistentFlags().Lookup("source-hostname"))
// 	viper.BindPFlag("TARGET_HOSTNAME", rootCmd.PersistentFlags().Lookup("target-hostname"))
// 	viper.BindPFlag("PACKAGE_TYPE", rootCmd.PersistentFlags().Lookup("package-type"))
// }

// func initConfig() {
// 	// Read in environment variables that match

// 	// Load .env file
// 	viper.SetConfigFile(".env")

// 	// Read .env file
// 	if err := viper.ReadInConfig(); err != nil {
// 		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
// 			// Config file not found, using only env vars
// 			fmt.Println("No .env file found, using environment variables only")
// 			panic(err)
// 		} else {
// 			// Config file was found but another error was produced
// 			fmt.Printf("Error reading config file: %v\n", err)
// 			panic(err)
// 		}
// 	}

// 	// Create a timestamp for the log file name
// 	timestamp := time.Now().Format("2006-01-02T15-04-05")

// 	// Define the log file path
// 	logFilePath := fmt.Sprintf("./migration-%s.log", timestamp)

// 	// Create the log file
// 	logFile, err := os.Create(logFilePath)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Failed to create log file: %v\n", err)
// 		os.Exit(1)
// 	}

// 	// Configure the logger to write to the file
// 	encoderConfig := zap.NewProductionEncoderConfig()
// 	encoderConfig.TimeKey = "timestamp"
// 	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
// 	core := zapcore.NewCore(
// 		zapcore.NewJSONEncoder(encoderConfig),
// 		zapcore.AddSync(logFile),
// 		zap.InfoLevel,
// 	)
// 	logger := zap.New(core)

// 	// Replace the global logger with the configured one
// 	zap.ReplaceGlobals(logger)
// }

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var rootCmd = &cobra.Command{
	Use:   "migrate-packages",
	Short: "gh cli extension to migrate packages between organizations",
	Long:  "gh cli extension to migrate packages between organizations",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Define root command flags
	rootCmd.PersistentFlags().String("http-proxy", "", "HTTP proxy")
	rootCmd.PersistentFlags().String("https-proxy", "", "HTTPS proxy")
	rootCmd.PersistentFlags().String("no-proxy", "", "No proxy list")
	rootCmd.PersistentFlags().Int("retry-max", 3, "Maximum retry attempts")
	rootCmd.PersistentFlags().String("retry-delay", "1s", "Delay between retries")

	// Bind flags to viper
	viper.BindPFlag("HTTP_PROXY", rootCmd.PersistentFlags().Lookup("http-proxy"))
	viper.BindPFlag("HTTPS_PROXY", rootCmd.PersistentFlags().Lookup("https-proxy"))
	viper.BindPFlag("NO_PROXY", rootCmd.PersistentFlags().Lookup("no-proxy"))
	viper.BindPFlag("RETRY_MAX", rootCmd.PersistentFlags().Lookup("retry-max"))
	viper.BindPFlag("RETRY_DELAY", rootCmd.PersistentFlags().Lookup("retry-delay"))

	// Add subcommands
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(syncCmd)

	// hide -h, --help from global/proxy flags
	rootCmd.Flags().BoolP("help", "h", false, "")
	rootCmd.Flags().Lookup("help").Hidden = true
}

func initConfig() {
	// Allow .env file
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	viper.SetConfigName(".env")

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Printf("Error reading config file: %v\n", err)
		}
	}

	// Read from environment
	viper.AutomaticEnv()

		// Create a timestamp for the log file name
	timestamp := time.Now().Format("2006-01-02T15-04-05")

	// Define the log file path
	logFilePath := fmt.Sprintf("./migration-%s.log", timestamp)

	// Create the log file
	logFile, err := os.Create(logFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log file: %v\n", err)
		os.Exit(1)
	}

	// Configure the logger to write to the file
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(logFile),
		zap.InfoLevel,
	)
	logger := zap.New(core)

	// Replace the global logger with the configured one
	zap.ReplaceGlobals(logger)
}
