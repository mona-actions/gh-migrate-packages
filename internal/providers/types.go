package providers

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/google/go-github/v62/github"
	"github.com/mona-actions/gh-migrate-packages/internal/utils"
	"github.com/shurcooL/githubv4"
	"go.uber.org/zap"
)

type MavenPackageStorageType []PackageNode

// ResultState represents the result of an operation
// e.g. downloading a file, etc
type ResultState int

const (
	Success ResultState = iota
	Skipped
	Failed
)

func (r ResultState) String() string {
	return [...]string{"Success", "Skipped", "Failed"}[r]
}

type BaseProvider struct {
	PackageType       string
	SourceRegistryUrl *url.URL
	TargetRegistryUrl *url.URL
	SourceHostnameUrl *url.URL
	TargetHostnameUrl *url.URL
}

type Provider interface {
	Connect(*zap.Logger) error
	FetchPackageFiles(*zap.Logger, string, string, string, string, string, *github.PackageMetadata) ([]string, ResultState, error)
	Export(*zap.Logger, string, interface{}) error
	Download(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error)
	Upload(logger *zap.Logger, owner, repository, packageType, packageName, version, filename string) (ResultState, error)
	GetDownloadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error)
	GetUploadUrl(logger *zap.Logger, owner, repository, packageName, version, filename string) (string, error)
	GetPackageType() string
}

func (p *BaseProvider) Export(logger *zap.Logger, owner string, content interface{}) error {
	if content == nil {
		return fmt.Errorf("source packages not fetched")
	}
	return nil
}

func (p *BaseProvider) GetPackageType() string {
	return p.PackageType
}

func Cache(path string, content []PackageNode) error {
	jsonBytes, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("error marshalling packages: %w", err)
	}
	_, err = utils.CacheFile(path, string(jsonBytes), true)
	return err
}

func LoadCache(path string) ([]PackageNode, error) {
	var content []PackageNode
	contentStr, err := utils.LoadCacheFile(path)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(contentStr), &content)
	if err != nil {
		return nil, err
	}
	return content, nil
}

type FileNode struct {
	Name githubv4.String
}

type FilesNode struct {
	Nodes    []FileNode
	PageInfo struct {
		EndCursor   githubv4.String
		HasNextPage bool
	}
}

type VersionNode struct {
	ID      githubv4.ID
	Version githubv4.String
	Files   FilesNode `graphql:"files(first: $filesFirst, after: $filesAfter)"`
}

type RepositoryNode struct {
	Name  githubv4.String
	Owner struct {
		Login githubv4.String
	}
}

type VersionsNode struct {
	Nodes    []VersionNode
	PageInfo struct {
		EndCursor   githubv4.String
		HasNextPage bool
	}
}

type PackageNode struct {
	ID          githubv4.ID
	Name        githubv4.String
	PackageType githubv4.String
	Repository  RepositoryNode `graphql:"repository"`
	Versions    VersionsNode   `graphql:"versions(first: $versionsFirst, after: $versionsAfter)"`
}

type Query struct {
	Organization struct {
		Packages struct {
			Nodes    []PackageNode
			PageInfo struct {
				EndCursor   githubv4.String
				HasNextPage bool
			}
		} `graphql:"packages(first: $packagesFirst, after: $packagesAfter, packageType: $packageType)"`
	} `graphql:"organization(login: $owner)"`
}

type VersionQuery struct {
	Node struct {
		Package struct {
			Versions struct {
				Nodes    []VersionNode
				PageInfo struct {
					EndCursor   githubv4.String
					HasNextPage bool
				}
			} `graphql:"versions(first: $versionsFirst, after: $versionsAfter)"`
		} `graphql:"... on Package"`
	} `graphql:"node(id: $packageID)"`
}

type FileQuery struct {
	Node struct {
		PackageVersion struct {
			Files struct {
				Nodes    []FileNode
				PageInfo struct {
					EndCursor   githubv4.String
					HasNextPage bool
				}
			} `graphql:"files(first: $filesFirst, after: $filesAfter)"`
		} `graphql:"... on PackageVersion"`
	} `graphql:"node(id: $versionID)"`
}
