package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func GetFlagOrEnv(cmd *cobra.Command, flags map[string]bool) map[string]string {
	values := make(map[string]string)
	var missing []string

	for name, required := range flags {
		// For CLI flags, strip GHMPKG_ prefix if present
		flagName := strings.TrimPrefix(strings.ToLower(name), "ghmpkg_")
		flagName = strings.ReplaceAll(flagName, "_", "-")

		// For env vars, ensure GHMPKG_ prefix
		envName := name
		if !strings.HasPrefix(strings.ToUpper(name), "GHMPKG_") {
			envName = "GHMPKG_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		}

		// Check all possible sources
		flagVal, _ := cmd.Flags().GetString(flagName)
		envVal := viper.GetString(envName)

		value := ""
		if flagVal != "" {
			value = flagVal
		} else if envVal != "" {
			value = envVal
		}

		if value != "" {
			// Store both versions to ensure consistency
			viper.Set(flagName, value)
			viper.Set(envName, value)
			values[name] = value
		} else if required {
			missing = append(missing, flagName)
		}
	}

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Error: missing required values: %s\n", strings.Join(missing, ", "))
		os.Exit(1)
	}

	return values
}

func ShowConnectionStatus(actionType string) {
	var endpoint string

	switch actionType {
	case "export", "pull":
		endpoint = "source-hostname"
	case "sync":
		endpoint = "target-hostname"
	}

	hostname := getNormalizedEndpoint(endpoint)

	fmt.Println(getHostnameMessage(hostname))
	//fmt.Println(getProxyStatus())
}

func getNormalizedEndpoint(key string) string {
	hostname := viper.GetString(key)
	if hostname != "" {
		hostname = strings.TrimPrefix(hostname, "http://")
		hostname = strings.TrimPrefix(hostname, "https://")
		hostname = strings.TrimSuffix(hostname, "/api/v3")
		hostname = strings.TrimSuffix(hostname, "/")
		hostname = fmt.Sprintf("https://%s/api/v3", hostname)
		viper.Set(key, hostname)
	}
	return hostname
}

func getHostnameMessage(hostname string) string {
	if hostname != "" {
		println(hostname)
		return fmt.Sprintf("\n💻 Using: GitHub Enterprise Server: %s", hostname)
	}
	return "\n🌍 Using: GitHub.com"
}

func getProxyStatus() string {
	if viper.GetString("HTTP_PROXY") != "" || viper.GetString("HTTPS_PROXY") != "" {
		return "✅ Proxy: Configured\n"
	}
	return "❎ Proxy: Not configured\n"
}
