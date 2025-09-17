# gh-migrate-packages

`gh-migrate-packages` is a [GitHub CLI](https://cli.github.com) extension to assist in the migration of packages between GitHub organizations and repositories. While GitHub Enterprise Importer handles many aspects of organization migration, there can be challenges with packages. This extension aims to fill the gaps in the existing solutions for migrating packages. Whether you are consolidating repositories in an organization or auditing packages in an existing organization, this extension can help.

## Install
```sh
gh extension install mona-actions/gh-migrate-packages
```

If you are are planning to migrate `containers` or `nuget` packages, you will also need to install the following tools installed.  

- [Docker](https://docs.docker.com/get-docker/)
- [.NET SDK](https://dotnet.microsoft.com/en-us/download)

## Upgrade
```sh
gh extension upgrade gh-migrate-packages
```


## Usage: Export

```sh
Usage:
  migrate-packages export [flags]

Flags:
  -h, --help                         help for export
  -p, --package-type string          Package type to export (optional)
  -h, --source-hostname string       GitHub Enterprise hostname (optional)
  -o, --source-organization string   Organization of the repository
  -t, --source-token string          GitHub token
```

Create a `csv` to prepare for migration. If you specify a package type or types, only those packages will be exported. For each package type a new file will be created. If you do not specify a package type, all packages will be exported into their own `csv` file.

### Example Export Command for all package types (recommended)
```sh
gh migrate-packages export \
  --source-organization mona-actions \
  --source-token ghp_xxxxxxxxxxxx
```

```sh
packages-migration
├── docker
│   └── 2025-01-11_12-00-00_mona-actions_docker_packages.csv
├── gem
│   └── 2025-01-11_12-00-00_mona-actions_gem_packages.csv
├── maven
│   └── 2025-01-11_12-00-00_mona-actions_maven_packages.csv
├── npm
│   └── 2025-01-11_12-00-00_mona-actions_npm_packages.csv
└── nuget
    └── 2025-01-11_12-00-00_mona-actions_nuget_packages.csv
```

### Example Export Command for specific package types

```sh
gh migrate-packages export \
  --package-type maven \
  --package-type nuget \
  --source-organization mona-actions \
  --source-token ghp_xxxxxxxxxxxx
```

```sh
package-migration
├── maven
│   └── 2025-01-11_12-00-00_mona-actions_maven_packages.csv
└── nuget
    └── 2025-01-11_12-00-00_mona-actions_nuget_packages.csv
```

If no package exist for a specific package type, the tool will not create a directory or file for that package type.

### Export summary

The export process provides additional feedback

```
📊 Export Summary:
Total packages found: 432
✅ Successfully processed: 432 packages
  📦 docker: 90
  📦 rubygem: 110
  📦 maven: 25
  📦 npm: 175
  📦 nuget: 32
❌ Failed to process: 0 packages
🔍 Repositories with packages: 246
📁 Output directory: packages-migration
🕐 Total time: 413s

✅ Export completed successfully!
```

## Usage: Pull

Pull packages from the source organization/repository to prepare for migration.

:warning: This could utilize signifgant disk space and network bandwidth. Ensure you have enough space and bandwidth to handle the pull operation.

```sh
Usage:
  migrate-packages pull [flags]

Flags:
  -h, --help                     help for pull
  -p, --package-type string      Package type to pull (optional)
  -n, --source-hostname string   GitHub Enterprise Server hostname URL (optional)
  -t, --source-token string      GitHub token with repo scope (required)
```
### Example Pull Command for all package types

```sh
gh migrate-packages pull \
  --source-token ghp_xxxxxxxxxxxx
```

### Example Pull Command for specific package types

```sh
gh migrate-packages pull \
  --package-type npm \
  --source-token ghp_xxxxxxxxxxxx
```
### Pull summary

```
📊 Summary:
✅ Successfully processed: 432 packages
  📦 docker: 90
  📦 rubygem: 110
  📦 maven: 25
  📦 npm: 175
  📦 nuget: 32
❌ Failed: 0 packages
📁 Output directory: package-migration/(npm, maven, nuget, rubygem, docker)
🕐 Total time: 1h 10m 10s

✅ Pull completed successfully!
```

## Usage: Sync

Push packages content to the target organization/repository.

```sh
Usage:
  migrate-packages sync [flags]

Flags:
  -h, --help                         help for sync
  -o, --source-organization string   Source Organization name (required)
  -p, --target-organization string   Target Organization to sync packages to (required)
  -t, --target-token string          Target Organization GitHub token. Scopes: admin:org (required)
  -m, --migration-path string        Path to the migration directory (default: ./migration-packages)
  -r, --repository string            Repository to sync (optional, syncs all repositories if not specified)
```

### Example Sync Command for all packages

```bash
gh migrate-packages sync \
  --source-organization mona-actions \
  --target-organization mona-emu \
  --target-token ghp_xxxxxxxxxxxx
```

### Example Sync Command with custom migration path

```bash
gh migrate-packages sync \
  --source-organization mona-actions \
  --target-organization mona-emu \
  --target-token ghp_xxxxxxxxxxxx \
  --migration-path /path/to/custom/migration/directory
```

### Example Sync Command for specific repository

```bash
gh migrate-packages sync \
  --source-organization mona-actions \
  --target-organization mona-emu \
  --target-token ghp_xxxxxxxxxxxx \
  --repository my-specific-repo
```

### Sync summary

```
📊 Summary:
✅ Successfully processed: 432 packages
  📦 docker: 90
  📦 rubygem: 110
  📦 maven: 25
  📦 npm: 175
  📦 nuget: 32
❌ Failed: 0 packages
📁 Input directory: package-migration/(npm, maven, nuget, rubygem, docker)
🕐 Total time: 1h 13m 27s

✅ Sync completed successfully!
```

## Updating Package Metadata

### RubyGems

The `Rename` method in the `RubyGemsProvider`(`internal/providers/gem.go`) performs two key replacements in the package `gemspec` file

1. Updates all references to the source organization's to point to the target organization
2. Updates all references to the source package registry URL to point to the target registry

This ensures that the gem's metadata correctly reflects its new location after migration. 

For example, if you're migrating from `old-org` to `new-org`, references like:
- `https://rubygems.pkg.github.com/old-org`
- `https://github.com/old-org`

Will be updated to:
- `https://rubygems.pkg.github.com/new-org`
- `https://github.com/new-org`

During the migration process, the tool will:
1. Extract the package contents
2. Update the gemspec file with the new organization scope
3. Republish the package to the new organization using gem push

### npm

The `Rename` method in the `NPMProvider`(`internal/providers/npm.go`) performs a single replacement in the package `package.json` file to reflect the new organization scope.

1. Updates `repository` references to the source organization to point to the target organization, specfically the `url` field if type is `git`.

```
{
  "repository": {
    "type": "git",
    "url": "git+https://github.com/mona-actions/npm-package.git"
  }
}
```

For example, if you're migrating from `old-org` to `new-org`, package names like:
- `@old-org/package-name`
- `https://npm.pkg.github.com/old-org`

Will be updated to:
- `@new-org/package-name`
- `https://npm.pkg.github.com/new-org`

During the migration process, the tool will:
1. Extract the package contents
2. Update the package.json with the new organization scope
3. Republish the package to the new organization using npm publish

### NuGet

When migrating NuGet packages, the tool performs some cleanup of the package metadata by removing specific files from the .nupkg archive to remove references to the source organization (`internal/providers/nuget.go`). The cleanup process is handled during the sync operation.

- `_rels/.rels`
- `[Content_Types].xml`

During the migration process, the tool will:
1. Remove the specified metadata files from the .nupkg archive
2. Push the package to the new organization using the GitHub Package Registry (GPR) tool

Note: Unlike RubyGems and NPM packages, NuGet packages do not require organization name updates in their metadata as they use a different naming convention.

### Docker

The `Rename` method in the `ContainerProvider` updates container image metadata to reflect the new organization:

1. Updates the `org.opencontainers.image.source` label to point to the new organization
2. Recreates the container image with updated metadata while preserving all other labels and configuration

For example, if you're migrating from `old-org` to `new-org`, labels like:
- `org.opencontainers.image.source=https://github.com/old-org/repo-name`

Will be updated to:
- `org.opencontainers.image.source=https://github.com/new-org/repo-name`

During the migration process, the tool will:
1. Pull the container image from the source registry
2. Create a new container with updated metadata
3. Commit the changes as a new image
4. Push the updated image to the target registry

Note: The tool maintains a cache of recreated image SHAs to optimize performance when the same image needs to be tagged multiple times.

## packages CSV Format

The tool exports and imports repository information using the following CSV format:

```csv
"organization", "repository", "type", "name", "version", "filename"
mona-actions,mona-actions-docker,docker,mona-actions-docker,1.0.0,mona-actions-docker-1.0.0.tar.gz
mona-actions,mona-actions-docker,docker,mona-actions-docker,1.0.1,mona-actions-docker-1.0.1.tar.gz
```

- `organization`: The name of the organization
- `repository`: The name of the repository
- `type`: The type of the package
- `name`: The name of the package
- `version`: The version of the package
- `filename`: The filename of the package

## Required Permissions

:warning: A personal access token with the `read:packages` and `repo` scopes is required for the export and pull operations. You cannot use a GitHub App token for these operations.

### For Export and Pull (Source Token)
- `read:packages` - Required for downloading packages
- `repo` - Required for accessing private repository packages

### For Sync (Target Token)
- `write:packages` - Required for publishing packages
- `delete:packages` - Required if replacing existing packages
- `repo` - Required for private repository access

## Environment Variables

The tool supports loading configuration from a `.env` file. This provides an alternative to command-line flags and allows you to store your configuration securely.

### Using a .env file

1. Create a `.env` file in your working directory:

```bash
# GitHub Migration PKG (GHMPKG)
GHMPKG_SOURCE_ORGANIZATION=mona-actions  # Source organization name
GHMPKG_SOURCE_HOSTNAME=                  # Source hostname
GHMPKG_SOURCE_TOKEN=ghp_xxx              # Source token
GHMPKG_TARGET_ORGANIZATION=mona-emu      # Target organization name
GHMPKG_TARGET_HOSTNAME=                  # Target hostname
GHMPKG_TARGET_TOKEN=ghp_yyy              # Target token
GHMPKG_PACKAGE_TYPE=npm                  # Package types to export (all, docker, rubygem, maven, npm, nuget)
GHMPKG_PACKAGE_TYPE=docker
GHMPKG_MIGRATION_PATH=./my-migration     # Custom migration directory path (default: ./migration-packages)
GHMPKG_REPOSITORY=my-specific-repo       # Specific repository to sync (optional)
```

2. Run the commands without flags - the tool will automatically load values from the .env file:

```bash
gh migrate-packages export
```
```bash
gh migrate-packages pull
```
```bash
gh migrate-packages sync
```

When both environment variables and command-line flags are provided, the command-line flags take precedence. This allows you to override specific values while still using the .env file for most configuration.

### Example with Mixed Usage

Load most values from .env but override the target organization

```bash
gh migrate-packages sync --target-organization different-org
```

## Retry Configuration

The tool includes configurable retry behavior for API calls:

```bash
Global Flags:
    --retry-delay string   Delay between retries (default "1s")
    --retry-max int        Maximum retry attempts (default 3)
```

Example usage with retry configuration:

```bash
gh migrate-packages export \
    --retry-max 5 \
    --retry-delay 2s
```

This configuration allows you to:
- Adjust the number of retry attempts for failed API calls
- Modify the delay between retry attempts
- Handle temporary API issues or rate limiting more gracefully

## Limitations
- This tool is designed to work with GitHub Packages. It does not currently support other package tools like Artifactory, Nexus, etc. In theory you could use the sync functionality to push packages to GitHub but that would require manual work.
- Network bandwidth and storage space should be considered when migrating large amounts of packages
- The tool will retry failed operations but may still encounter persistent access or network issues

## :warning: Disclaimers
- If you change your organization name, and opt in to metadata changes, your package metadata will be updated to reflect the new organization. Opting out can/will result in package metadata pointing to the wrong organization name which can have significant impact downstream (e.g. build failures).
- If your package build produces checksums (e.g. `maven`), and you've made an organization name change which resulted in a package metadate update, you may need to update the checksums in your packages. Please work with GitHub Professional Services to see what solutions are available to you.

## Open Source

This tool uses `gpr` to push nuget packages which was written by GitHub staff and other contributors. For more information on `gpr` see https://github.com/jcansdale/gpr

## License

- [MIT](./license) (c) [Mona-Actions](https://github.com/mona-actions)
- [Contributing](./contributing.md)
