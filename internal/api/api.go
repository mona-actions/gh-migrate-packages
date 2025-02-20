package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-github/v62/github"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

type Releases []Release

type Release struct {
	*github.RepositoryRelease
}

type ProxyConfig struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}

var tmpDir = "tmp"

// Helper function to handle optional hostname parameter
func getHostname(hostname ...string) string {
	if len(hostname) > 0 {
		return hostname[0]
	}
	return ""
}

func newGitHubClientWithHostname(token string, hostname string) (*github.Client, error) {
	client, err := newGitHubClientWithProxy(token, GetProxyConfigFromEnv())
	if err != nil {
		return nil, err
	}

	if hostname == "" {
		return client, nil
	}

	baseURL, err := url.Parse(hostname)
	if err != nil {
		return nil, fmt.Errorf("invalid hostname URL provided (%s): %w", baseURL, err)
	}

	enterpriseClient, err := client.WithEnterpriseURLs(hostname, hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to configure enterprise URLs for %s: %w", hostname, err)
	}

	return enterpriseClient, nil
}

func newGitHubClientWithProxy(token string, proxyConfig *ProxyConfig) (*github.Client, error) {
	if token == "" {
		return nil, fmt.Errorf("GitHub token is required")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			if proxyConfig != nil && proxyConfig.NoProxy != "" {
				noProxyURLs := strings.Split(proxyConfig.NoProxy, ",")
				reqHost := req.URL.Host
				for _, noProxy := range noProxyURLs {
					if strings.TrimSpace(noProxy) == reqHost {
						return nil, nil
					}
				}
			}

			if proxyConfig != nil {
				if req.URL.Scheme == "https" && proxyConfig.HTTPSProxy != "" {
					return url.Parse(proxyConfig.HTTPSProxy)
				}
				if req.URL.Scheme == "http" && proxyConfig.HTTPProxy != "" {
					return url.Parse(proxyConfig.HTTPProxy)
				}
			}
			return nil, nil
		},
	}

	tc := oauth2.NewClient(ctx, ts)
	tc.Transport = &oauth2.Transport{
		Base:   transport,
		Source: ts,
	}

	return github.NewClient(tc), nil
}

func GetProxyConfigFromEnv() *ProxyConfig {
	return &ProxyConfig{
		HTTPProxy:  viper.GetString("HTTP_PROXY"),
		HTTPSProxy: viper.GetString("HTTPS_PROXY"),
		NoProxy:    viper.GetString("NO_PROXY"),
	}
}

func retryOperation(operation func() error) error {
	maxRetries := viper.GetInt("MAX_RETRIES")
	if maxRetries <= 0 {
		maxRetries = 3 // fallback default
	}

	retryDelay, err := time.ParseDuration(viper.GetString("RETRY_DELAY"))
	if err != nil {
		retryDelay = time.Second // fallback default
	}

	var apiErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		apiErr = operation()
		if apiErr == nil {
			return nil
		}

		if attempt < maxRetries {
			waitTime := retryDelay * time.Duration(1<<uint(attempt-1))
			fmt.Printf("Attempt %d failed, retrying in %v: %v\n", attempt, waitTime, apiErr)
			time.Sleep(waitTime)
		}
	}
	return apiErr
}

func FetchPackages(packageType string) ([]*github.Package, error) {
	client, err := newGitHubClientWithHostname(viper.GetString("GHMPKG_SOURCE_TOKEN"), "")
	ctx := context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)
	state := "active"
	var packages []*github.Package
	var page int

	err = retryOperation(func() error {
		packages = nil
		page = 1

		for {
			packagesPage, response, err := client.Organizations.ListPackages(ctx, viper.GetString("GHMPKG_SOURCE_ORGANIZATION"), &github.PackageListOptions{
				PackageType: &packageType,
				State:       &state,
				ListOptions: github.ListOptions{PerPage: 100, Page: page},
			})

			if err != nil {
				return err
			}

			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("error fetching packages: %v", response.Body)
			}

			packages = append(packages, packagesPage...)

			if response.NextPage == 0 {
				break
			}

			page = response.NextPage
		}

		return nil
	})

	return packages, err
}

func FetchPackageVersions(pkg *github.Package) ([]*github.PackageVersion, error) {
	client, err := newGitHubClientWithHostname(viper.GetString("GHMPKG_SOURCE_TOKEN"), getHostname(""))
	ctx := context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)
	state := "active"
	var versions []*github.PackageVersion
	var page int

	err = retryOperation(func() error {
		versions = nil
		page = 1

		for {
			versionsPage, response, err := client.Organizations.PackageGetAllVersions(ctx, viper.GetString("GHMPKG_SOURCE_ORGANIZATION"), *pkg.PackageType, *pkg.Name, &github.PackageListOptions{
				PackageType: pkg.PackageType,
				State:       &state,
				ListOptions: github.ListOptions{PerPage: 100, Page: page},
			})

			if err != nil {
				return err
			}

			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("error fetching versions: %v", response.Body)
			}

			versions = append(versions, versionsPage...)

			if response.NextPage == 0 {
				break
			}

			page = response.NextPage
		}

		return nil
	})

	return versions, err
}

func PackageExists(packageName, packageType string) (bool, error) {
	client, err := newGitHubClientWithHostname(viper.GetString("GHMPKG_TARGET_TOKEN"), "")
	ctx := context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)

	var exists = true
	err = retryOperation(func() error {
		_, response, err := client.Organizations.GetPackage(ctx, viper.GetString("GHMPKG_TARGET_ORGANIZATION"), packageType, packageName)

		if response.StatusCode != http.StatusOK {
			exists = false
			return nil
		}
		return err
	})

	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	return true, nil
}
