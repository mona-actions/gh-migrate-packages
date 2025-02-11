package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v62/github"
	"github.com/mona-actions/gh-migrate-packages/internal/files"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

type Releases []Release

type Release struct {
	*github.RepositoryRelease
}

var tmpDir = "tmp"

func newGHRestClient(token string, hostname string) *github.Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	rateLimiter, err := github_ratelimit.NewRateLimitWaiterClient(tc.Transport)

	if err != nil {
		panic(err)
	}

	client := github.NewClient(rateLimiter)

	if hostname != "" {
		hostname = strings.TrimSuffix(hostname, "/")
		client, err = github.NewClient(rateLimiter).WithEnterpriseURLs("https://"+hostname+"/api/v3", "https://"+hostname+"/api/uploads")
		if err != nil {
			panic(err)
		}
	}

	return client
}

func FetchPackages(packageType string) ([]*github.Package, error) {
	client := newGHRestClient(viper.GetString("GHMPKG_SOURCE_TOKEN"), "")
	ctx := context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)
	state := "active"
	var packages []*github.Package

	for {
		packagesPage, response, err := client.Organizations.ListPackages(ctx, viper.GetString("GHMPKG_SOURCE_ORGANIZATION"), &github.PackageListOptions{
			PackageType: &packageType,
			State:       &state,
			ListOptions: github.ListOptions{PerPage: 10},
		})

		if err != nil {
			return nil, err
		}

		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("error fetching packages: %v", response.Body)
		}

		packages = append(packages, packagesPage...)

		if response.NextPage == 0 {
			break
		}

	}

	return packages, nil
}

func FetchPackageVersions(pkg *github.Package) ([]*github.PackageVersion, error) {
	client := newGHRestClient(viper.GetString("GHMPKG_SOURCE_TOKEN"), "")
	ctx := context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)
	state := "active"
	var versions []*github.PackageVersion

	for {
		versionsPage, response, err := client.Organizations.PackageGetAllVersions(ctx, viper.GetString("GHMPKG_SOURCE_ORGANIZATION"), *pkg.PackageType, *pkg.Name, &github.PackageListOptions{
			PackageType: pkg.PackageType,
			State:       &state,
			ListOptions: github.ListOptions{PerPage: 100},
		})

		if err != nil {
			return nil, err
		}

		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("error fetching versions: %v", response.Body)
		}

		versions = append(versions, versionsPage...)

		if response.NextPage == 0 {
			break
		}
	}

	return versions, nil
}

func PackageExists(packageName, packageType string) (bool, error) {
	client := newGHRestClient(viper.GetString("GHMPKG_TARGET_TOKEN"), "")
	ctx := context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)

	_, response, err := client.Organizations.GetPackage(ctx, viper.GetString("TARGET_ORGANIZATION"), packageType, packageName)
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// func GetSourcePackages(owner string, packageType string) ([]types.PackageNode, error) {
// }

func DownloadReleaseAssets(asset *github.PackageFile) error {

	token := viper.Get("GHMPKG_SOURCE_TOKEN").(string)

	// Download the asset

	url := asset.GetDownloadURL()
	dirName := tmpDir
	fileName := dirName + "/" + asset.GetName()

	err := os.MkdirAll(dirName, 0755)
	if err != nil {
		return err
	}

	err = DownloadFileFromURL(url, fileName, token)
	if err != nil {
		return err
	}
	return nil
}

func DownloadReleaseZip(release *github.RepositoryRelease) error {
	token := viper.Get("GHMPKG_SOURCE_TOKEN").(string)
	repo := viper.Get("REPOSITORY").(string)
	if release.TagName == nil {
		return errors.New("TagName is nil")
	}
	tag := *release.TagName
	var tagName string

	url := *release.ZipballURL

	if len(tag) > 1 && tag[0] == 'v' && unicode.IsDigit(rune(tag[1])) {
		tagName = strings.TrimPrefix(tag, "v")
	} else {
		tagName = tag
	}

	fileName := fmt.Sprintf("%s-%s.zip", repo, tagName)

	err := DownloadFileFromURL(url, fileName, token)
	if err != nil {
		return err
	}

	return nil
}

func DownloadReleaseTarball(release *github.RepositoryRelease) error {
	token := viper.Get("GHMPKG_SOURCE_TOKEN").(string)
	repo := viper.Get("REPOSITORY").(string)
	if release.TagName == nil {
		return errors.New("TagName is nil")
	}
	tag := *release.TagName
	var tagName string

	url := *release.TarballURL

	if len(tag) > 1 && tag[0] == 'v' && unicode.IsDigit(rune(tag[1])) {
		tagName = strings.TrimPrefix(tag, "v")
	} else {
		tagName = tag
	}

	fileName := fmt.Sprintf("%s-%s.tar.gz", repo, tagName)

	err := DownloadFileFromURL(url, fileName, token)
	if err != nil {
		return err
	}

	return nil
}

func DownloadFileFromURL(url, fileName, token string) error {
	// Create the file
	out, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer out.Close()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(fmt.Errorf("error creating request: %s", err))
	}

	req.Header.Add("Authorization", "Bearer "+token)

	// Get the data
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error getting file: %v  err:%v", fileName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status code %d, Message: %s", resp.StatusCode, resp.Body)
	}

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func DoesPackageExist(owner string, pkg *github.Package) (*github.Package, error) {
	client := newGHRestClient(viper.GetString("GHMPKG_TARGET_TOKEN"), "")

	ctx := context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)
	var dstPkg *github.Package
	var response *github.Response
	var err error
	dstPkg, response, err = client.Organizations.GetPackage(ctx, owner, *pkg.PackageType, *pkg.Name)
	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return dstPkg, nil
}

func UploadAssetViaURL(uploadURL string, asset *github.ReleaseAsset) error {

	dirName := tmpDir
	fileName := dirName + "/" + asset.GetName()

	// Open the file
	file, err := files.OpenFile(fileName)
	if err != nil {
		return fmt.Errorf("error opening file: %v err: %v", file, err)
	}

	// Get the file size
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("error getting file size of %v err: %v ", fileName, err)
	}

	// Get the media type
	mediaType := mime.TypeByExtension(filepath.Ext(file.Name()))
	if *asset.ContentType != "" {
		mediaType = asset.GetContentType()
	}

	uploadURL = strings.TrimSuffix(uploadURL, "{?name,label}")

	// Add the name and label to the URL
	params := url.Values{}
	params.Add("name", asset.GetName())
	params.Add("label", asset.GetLabel())

	uploadURLWithParams := fmt.Sprintf("%s?%s", uploadURL, params.Encode())

	// Create the request
	req, err := http.NewRequest("POST", uploadURLWithParams, file)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	// Set the headers
	req.ContentLength = stat.Size()
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+viper.Get("TARGET_TOKEN").(string))
	req.Header.Set("Content-Type", mediaType)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error uploading asset to release: %v err: %v", uploadURL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("error uploading asset to release: %v err: %v", uploadURL, resp.Body)
	}

	err = files.RemoveFile(fileName)
	if err != nil {
		return fmt.Errorf("error deleting asset from local storage: %v err: %v", asset.Name, err)
	}

	return nil
}

func WriteToIssue(owner string, repository string, issueNumber int, comment string) error {

	client := newGHRestClient(viper.GetString("GHMPKG_TARGET_TOKEN"), "")

	ctx := context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)
	_, _, err := client.Issues.CreateComment(ctx, owner, repository, issueNumber, &github.IssueComment{Body: &comment})
	if err != nil {
		return err
	}

	return nil
}

func GetDatafromGitHubContext() (string, string, int, error) {
	githubContext := os.Getenv("GITHUB_CONTEXT")
	if githubContext == "" {
		return "", "", 0, fmt.Errorf("GITHUB_CONTEXT is not set or empty")
	}

	var issueEvent github.IssueEvent

	err := json.Unmarshal([]byte(githubContext), &issueEvent)
	if err != nil {
		return "", "", 0, fmt.Errorf("error unmarshalling GITHUB_CONTEXT: %v", err)
	}
	organization := *issueEvent.Repository.Owner.Login
	repository := *issueEvent.Repository.Name
	issueNumber := *issueEvent.Issue.Number

	return organization, repository, issueNumber, nil
}
