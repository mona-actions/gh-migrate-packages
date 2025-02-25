package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/google/go-github/v62/github"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Package providers implements different package type handlers for container registries.

// ContainerProvider handles container image operations between Docker registries.
// It supports pulling images from a source registry and pushing them to a target registry,
// while managing authentication and image metadata.
type ContainerProvider struct {
	BaseProvider
	ctx           context.Context
	client        *client.Client
	sourceAuthStr string
	targetAuthStr string
	recreatedShas map[string]string
}

// Constructor
// ----------

// NewContainerProvider creates a new ContainerProvider instance.
func NewContainerProvider(logger *zap.Logger, packageType string) Provider {
	return &ContainerProvider{
		BaseProvider:  NewBaseProvider(packageType, "", "", true),
		recreatedShas: make(map[string]string),
	}
}

// Authentication
// -------------

// encodeAuthToBase64 converts Docker registry authentication config to base64 encoded string.
func encodeAuthToBase64(auth registry.AuthConfig) (string, error) {
	authBytes, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(authBytes), nil
}

// login authenticates with a Docker registry and returns the encoded auth string.
func (p *ContainerProvider) login(logger *zap.Logger, addr, username, password string) (string, error) {
	auth := registry.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: addr,
	}
	authStr, err := encodeAuthToBase64(auth)
	if err != nil {
		return "", err
	}

	loginResp, err := p.client.RegistryLogin(p.ctx, auth)
	if err != nil {
		logger.Error("Failed to login to registry", zap.String("addr", addr), zap.Error(err))
		return "", err
	}

	if loginResp.Status != "Login Succeeded" {
		logger.Error("Failed to login to registry", zap.String("addr", addr), zap.String("status", loginResp.Status))
		return "", fmt.Errorf("failed to login to registry: %s", loginResp.Status)
	}

	return authStr, nil
}

// Connect initializes the Docker client and authenticates with both source and target registries.
func (p *ContainerProvider) Connect(logger *zap.Logger) error {
	// Add validation for required environment variables
	sourceOrg := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	sourceToken := viper.GetString("GHMPKG_SOURCE_TOKEN")

	if sourceOrg == "" || sourceToken == "" {
		return fmt.Errorf("missing required environment variables: GHMPKG_SOURCE_ORGANIZATION and/or GHMPKG_SOURCE_TOKEN")
	}

	ctx := context.Background()

	// Create Docker client
	client, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		logger.Error("Failed to create docker client", zap.Error(err))
		return err
	}

	p.ctx = ctx
	p.client = client

	if sourceOrg != "" && sourceToken != "" {
		sourceAuthStr, err := p.login(logger, p.SourceRegistryUrl.String(), sourceOrg, sourceToken)
		if err != nil {
			logger.Error("Failed to login to source registry", zap.Error(err))
			return err
		}
		p.sourceAuthStr = sourceAuthStr
	}

	targetOrg := viper.GetString("GHMPKG_TARGET_ORGANIZATION")
	targetToken := viper.GetString("GHMPKG_TARGET_TOKEN")
	if targetOrg != "" && targetToken != "" { //if targetOrg and token are empty, we don't need to login
		targetAuthStr, err := p.login(logger, p.TargetRegistryUrl.String(), targetOrg, targetToken)
		if err != nil {
			logger.Error("Failed to login to target registry", zap.Error(err))
			return err
		}
		p.targetAuthStr = targetAuthStr
	}

	return nil
}

// Core Operations
// --------------

// FetchPackageFiles retrieves the list of container image tags for a package.
func (p *ContainerProvider) FetchPackageFiles(logger *zap.Logger, owner, repository, packageType, packageName, version string, metadata *github.PackageMetadata) ([]string, ResultState, error) {
	filenames := []string{}
	for _, tag := range metadata.Container.Tags {
		filenames = append(filenames, fmt.Sprintf("%s:%s", packageName, tag))
	}
	// Reverse the slice to upload the latest version last
	for i := 0; i < len(filenames)/2; i++ {
		j := len(filenames) - 1 - i
		filenames[i], filenames[j] = filenames[j], filenames[i]
	}
	return filenames, Success, nil
}

// Download pulls a container image from the source registry and saves it locally.
func (p *ContainerProvider) Download(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
	// Normalize names for container images
	owner, repository, packageName = p.normalizeNames(owner, repository, packageName)

	parts := strings.Split(filename, ":")
	tag := parts[1]
	downloadedFilename := fmt.Sprintf("%s-%s.tar", packageName, tag)

	return p.downloadPackage(
		logger, owner, repository, packageType, packageName, version, filename, &downloadedFilename,
		// URL generator function
		func() (string, error) {
			return p.GetDownloadUrl(logger, owner, repository, packageName, version, filename)
		},
		// Download function
		func(downloadUrl, outputPath string) (ResultState, error) {
			pullResp, err := p.client.ImagePull(p.ctx, downloadUrl, image.PullOptions{
				RegistryAuth: p.sourceAuthStr,
			})
			if err != nil {
				logger.Error("Failed to pull image",
					zap.String("package", packageName),
					zap.String("version", version),
					zap.String("image", downloadUrl),
					zap.Error(err))
				return Failed, err
			}
			defer pullResp.Close()

			// Must read the response to complete the pull
			if _, err = io.Copy(io.Discard, pullResp); err != nil {
				logger.Error("Failed to read pull response",
					zap.String("image", downloadUrl),
					zap.Error(err))
				return Failed, err
			}

			// Save image to file
			saveResp, err := p.client.ImageSave(p.ctx, []string{downloadUrl})
			if err != nil {
				logger.Error("Failed to save image",
					zap.String("image", downloadUrl),
					zap.String("output", outputPath),
					zap.Error(err))
				return Failed, err
			}
			defer saveResp.Close()

			// Create output file
			outputFile, err := os.Create(outputPath)
			if err != nil {
				logger.Error("Failed to create output file",
					zap.String("path", outputPath),
					zap.Error(err))
				return Failed, err
			}
			defer outputFile.Close()

			// Copy image to file
			if _, err = io.Copy(outputFile, saveResp); err != nil {
				logger.Error("Failed to write image to file",
					zap.String("path", outputPath),
					zap.String("image", downloadUrl),
					zap.Error(err))
				return Failed, err
			}
			return Success, nil
		},
	)
}

// Rename creates a new image with updated metadata for the target registry.
func (p *ContainerProvider) Rename(logger *zap.Logger, owner, repository, packageName, version, filename string) error {
	// Skip if source and target organizations are the same
	if p.CheckOrganizationsMatch(logger) {
		return nil
	}

	// Tag image for target registry
	sourceOrg := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	targetOrg := viper.GetString("GHMPKG_TARGET_ORGANIZATION")
	sourceRef, err := p.GetDownloadUrl(logger, sourceOrg, repository, packageName, version, filename)
	if err != nil {
		logger.Error("Failed to get download URL", zap.Error(err))
		return err
	}
	targetRef, err := p.GetUploadUrl(logger, targetOrg, repository, packageName, version, filename)
	if err != nil {
		logger.Error("Failed to get upload URL", zap.Error(err))
		return err
	}

	// Get existing image details
	inspect, _, err := p.client.ImageInspectWithRaw(p.ctx, sourceRef)
	if err != nil {
		return fmt.Errorf("failed to inspect image: %w", err)
	}

	// check if inspect.ID is in recreatedShas
	if origTargetRef, ok := p.recreatedShas[inspect.ID]; ok {
		err = p.client.ImageTag(p.ctx, origTargetRef, targetRef)
		if err != nil {
			logger.Error("Failed to tag image", zap.Error(err))
			return err
		}
		return nil
	}

	// Create new labels map, copying existing labels
	newLabels := make(map[string]string)
	for k, v := range inspect.Config.Labels {
		newLabels[k] = v
	}

	// Update the specific label
	newLabels["org.opencontainers.image.source"] = strings.Replace(
		newLabels["org.opencontainers.image.source"],
		sourceOrg,
		targetOrg,
		1,
	)

	// Create a container with the new labels
	resp, err := p.client.ContainerCreate(p.ctx, &container.Config{
		Image:  sourceRef,
		Labels: newLabels,
	}, nil, nil, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}
	defer p.client.ContainerRemove(p.ctx, resp.ID, container.RemoveOptions{})

	// Commit the container as a new image
	_, err = p.client.ContainerCommit(p.ctx, resp.ID, container.CommitOptions{
		Reference: targetRef,
		Config: &container.Config{
			Labels: newLabels,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit container: %w", err)
	}

	p.recreatedShas[inspect.ID] = targetRef

	return nil
}

// Upload pushes a container image to the target registry.
func (p *ContainerProvider) Upload(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
	// Normalize names for container images
	owner, repository, packageName = p.normalizeNames(owner, repository, packageName)

	return p.uploadPackage(
		logger, owner, repository, packageType, packageName, version, filename,
		func() (string, error) {
			return p.GetUploadUrl(logger, owner, repository, packageName, version, filename)
		},
		func(uploadUrl, packageDir string) (ResultState, error) {
			if err := p.Rename(logger, owner, repository, packageName, version, filename); err != nil {

				logger.Error("Failed to rename image", zap.Error(err))
				return Failed, err
			}
			targetRef, err := p.GetUploadUrl(logger, viper.GetString("GHMPKG_TARGET_ORGANIZATION"), repository, packageName, version, filename)
			if err != nil {
				logger.Error("Failed to get upload URL", zap.Error(err))
				return Failed, err
			}
			// Push image to target registry
			pushResp, err := p.client.ImagePush(p.ctx, targetRef, image.PushOptions{
				RegistryAuth: p.targetAuthStr,
			})
			if err != nil {
				logger.Error("Failed to push image", zap.Error(err))
				return Failed, err
			}
			defer pushResp.Close()

			// Must read the response to complete the push
			_, err = io.Copy(io.Discard, pushResp)
			if err != nil {
				logger.Error("Failed to read push response", zap.Error(err))
				return Failed, err
			}
			return Success, nil
		},
	)
}

// URL Generation
// -------------

// GetFetchUrl generates the URL for fetching package metadata.
func (p *ContainerProvider) GetFetchUrl(logger *zap.Logger, owner, packageName, version string) (string, error) {
	fetchUrl := *p.SourceRegistryUrl
	fetchUrl.Path = path.Join(fetchUrl.Path, fmt.Sprintf("@%s", owner), packageName)
	return fetchUrl.String(), nil
}

// GetDownloadUrl generates the URL for downloading a container image from the source registry.
func (p *ContainerProvider) GetDownloadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error) {
	// Normalize names for container images
	owner, repository, packageName = p.normalizeNames(owner, repository, packageName)

	downloadUrl := *p.SourceRegistryUrl
	downloadUrl.Path = path.Join(downloadUrl.Path, owner, filename)
	return downloadUrl.String(), nil
}

// GetUploadUrl generates the URL for uploading a container image to the target registry.
func (p *ContainerProvider) GetUploadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error) {
	// Normalize names for container images
	owner, repository, packageName = p.normalizeNames(owner, repository, packageName)

	uploadUrl := *p.TargetRegistryUrl
	uploadUrl.Path = path.Join(uploadUrl.Path, owner, filename)
	return uploadUrl.String(), nil
}

// Required Interface Methods
// ------------------------

// Export implements the Provider interface Export method.
func (p *ContainerProvider) Export(logger *zap.Logger, owner string, content interface{}) error {
	return p.BaseProvider.Export(logger, owner, content)
}

// Add these methods near the top of the ContainerProvider struct methods

func (p *ContainerProvider) normalizeNames(owner, repository, packageName string) (string, string, string) {
	return strings.ToLower(owner),
		strings.ToLower(repository),
		strings.ToLower(packageName)
}
