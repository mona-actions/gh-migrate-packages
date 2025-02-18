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

type ContainerProvider struct {
	BaseProvider
	ctx           context.Context
	client        *client.Client
	sourceAuthStr string
	targetAuthStr string
	recreatedShas map[string]string
}

func NewContainerProvider(logger *zap.Logger, packageType string) Provider {
	return &ContainerProvider{
		BaseProvider:  NewBaseProvider(packageType, "", "", true),
		recreatedShas: make(map[string]string),
	}
}

// Helper function to encode auth config to base64
func encodeAuthToBase64(auth registry.AuthConfig) (string, error) {
	authBytes, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(authBytes), nil
}

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

func (p *ContainerProvider) Connect(logger *zap.Logger) error {
	ctx := context.Background()

	// Create Docker client
	client, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		logger.Error("Failed to create docker client", zap.Error(err))
		return err
	}

	p.ctx = ctx
	p.client = client

	sourceOrg := viper.GetString("GHMPKG_SOURCE_ORGANIZATION")
	sourceToken := viper.GetString("GHMPKG_SOURCE_TOKEN")
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

func (p *ContainerProvider) FetchPackageFiles(logger *zap.Logger, owner, repository, packageType, packageName, version string, metadata *github.PackageMetadata) ([]string, ResultState, error) {
	filenames := []string{}
	for _, tag := range metadata.Container.Tags {
		filenames = append(filenames, fmt.Sprintf("%s:%s", packageName, tag))
	}
	return filenames, Success, nil
}

func (p *ContainerProvider) Export(logger *zap.Logger, owner string, content interface{}) error {
	return p.BaseProvider.Export(logger, owner, content)
}

func (p *ContainerProvider) Download(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
	parts := strings.Split(filename, ":")
	tag := parts[1]
	downloadedFilename := fmt.Sprintf("%s-%s.tar", packageName, tag)

	//lowercase owner, repository and packageName to avoid issues with docker
	owner = strings.ToLower(owner)
	repository = strings.ToLower(repository)
	packageName = strings.ToLower(packageName)

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
				logger.Error("Failed to pull image", zap.Error(err))
				return Failed, err
			}
			defer pullResp.Close()

			// Must read the response to complete the pull
			_, err = io.Copy(io.Discard, pullResp)
			if err != nil {
				logger.Error("Failed to read pull response", zap.Error(err))
				return Failed, err
			}

			// Save image to file
			saveResp, err := p.client.ImageSave(p.ctx, []string{downloadUrl})
			if err != nil {
				logger.Error("Failed to save image", zap.Error(err))
				return Failed, err
			}
			defer saveResp.Close()

			// Create output file
			outputFile, err := os.Create(outputPath)
			if err != nil {
				logger.Error("Failed to create output file", zap.Error(err))
				return Failed, err
			}
			defer outputFile.Close()

			// Copy image to file
			_, err = io.Copy(outputFile, saveResp)
			if err != nil {
				logger.Error("Failed to write image to file", zap.Error(err))
				return Failed, err
			}
			return Success, nil
		},
	)
}

func (p *ContainerProvider) Rename(logger *zap.Logger, owner, repository, packageName, version, filename string) error {
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

func (p *ContainerProvider) Upload(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error) {
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

func (p *ContainerProvider) GetFetchUrl(logger *zap.Logger, owner, packageName, version string) (string, error) {
	fetchUrl := *p.SourceRegistryUrl
	fetchUrl.Path = path.Join(fetchUrl.Path, fmt.Sprintf("@%s", owner), packageName)
	return fetchUrl.String(), nil
}

func (p *ContainerProvider) GetDownloadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error) {
	downloadUrl := *p.SourceRegistryUrl
	downloadUrl.Path = path.Join(downloadUrl.Path, owner, filename)
	return downloadUrl.String(), nil
}

func (p *ContainerProvider) GetUploadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error) {
	uploadUrl := *p.TargetRegistryUrl
	uploadUrl.Path = path.Join(uploadUrl.Path, owner, filename)
	return uploadUrl.String(), nil
}
