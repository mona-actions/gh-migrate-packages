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
	// rootCmd.PersistentFlags().String("http-proxy", "", "HTTP proxy")
	// rootCmd.PersistentFlags().String("https-proxy", "", "HTTPS proxy")
	// rootCmd.PersistentFlags().String("no-proxy", "", "No proxy list")
	rootCmd.PersistentFlags().Int("retry-max", 3, "Maximum retry attempts")
	rootCmd.PersistentFlags().String("retry-delay", "1s", "Delay between retries")

	// Bind flags to viper
	// viper.BindPFlag("HTTP_PROXY", rootCmd.PersistentFlags().Lookup("http-proxy"))
	// viper.BindPFlag("HTTPS_PROXY", rootCmd.PersistentFlags().Lookup("https-proxy"))
	// viper.BindPFlag("NO_PROXY", rootCmd.PersistentFlags().Lookup("no-proxy"))
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

	// Define the log directory and file path
	logDir := "./migration-packages/logs"
	logFilePath := fmt.Sprintf("%s/%s.log", logDir, timestamp)

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
		os.Exit(1)
	}

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
