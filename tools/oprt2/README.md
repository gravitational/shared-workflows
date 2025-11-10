# OPRT2

The OS Package Repo Tool 2 (OPRT2) is a tool for managing release packages
within repositories.

## Features

* Repo types
  * APT
  * ~~YUM~~ (planned, not implemented yet)
* Repo managers
  * [Attune](https://github.com/attunehq/attune)
    * ~~Authentication sources~~ (planned, not implemented yet)
      * Token/`Authorization` header
      * mTLS (requires reverse proxy in front of Attune control plane)
        * ~~Certificate providers~~ (planned, not implemented yet)
          * Teleport Machine Workload Identity
          * Pre-existing PEM-encoded keypairs
  * Discard (dry-run/no-op)
* File sources
  * Local filesystem
  * S3 bucket
* ~~GPG sources~~ (planned, not implemented yet)
  * Pre-existing `.gnupg` directory
  * Base64-encoded tarball of a `.gnupg` directory

## Example config

```yaml
---
# yaml-language-server: $schema=./config.json
logger:
  level: debug
packageManagers:
  - apt:
      fileSource:
        s3:
          bucket: some-bucket-name  # TODO this will need to support coming from env var because it is enviroment-specific
          path: teleport/tags/
      publishingTool:
        attune:
          gpg:
            # Base-64 encoded tarball of `.gnupg` directory, matching what we've done for years
            # Example command to generate this: `tar --exclude='.#*' --exclude="*~" --exclude="S.*" -czf - ~/.gnupg/ | base64`
            # TODO this will need to support coming from env var because it is enviroment-specific
            archive: c29tZSBiYXNlNjQtZW5jb2RlZCB0YXJiYWxsCg==
          authentication:
            mTLS:
            endpoint: https://attune.ci-cd-cluster.tld
            certificateSource:
              teleport:
              workloadIdentity:
                name: workload-id
      # When adding a new supported OS, update the github.com/gravitational/teleport/blob/master/lib/web/scripts/node-join/install.sh script
      # Otherwise, it will keep using the binary installation instead of the deb repo.
      repos:
        # https://wiki.ubuntu.com/Releases for details
        ubuntu:
          # Anchors are used for brevity, but don't need to be
          xenial: &older_distros # 16.04 LTS
            stable/v18: &major_version_files
              - teleport-(amd|arm|i386|arm)64\.deb
              - teleport-updater\.deb
              - other-packages\.deb
            stable/cloud:
              - teleport-amd64\.deb
            stable/rolling: *major_version_files
          bionic: *older_distros # 18.04 LTS
          focal: *older_distros # 20.04 LTS
          jammy: &newer_distros # 22.04 LTS
            stable/v18: &major_version_files
              # No i386 or arm
              - teleport-(amd|arm)64\.deb
              - teleport-updater\.deb
              - other-packages\.deb
            stable/cloud:
              - teleport-amd64\.deb
            stable/rolling: *major_version_files
          noble: *newer_distros # 24.04 LTS
          plucky: *newer_distros # 25.04
        # See https://wiki.debian.org/DebianReleases#Production_Releases for details
        debian:
          bullseye: *older_distros # 11
          bookworm: *older_distros # 12
          trixie: *newer_distros # 13
          forky: *newer_distros # 14
        stable/rolling: *major_version_files
```

## Usage as a library

The primary entrypoint for this library is [ospackages.Manager] implementations,
such as [apt.NewManager]. Managers provide methods for manipulating package
repositories. Here's a truncated example:

```go
repos := map[string]map[string]map[string][]*regexp.Regexp{
	"ubuntu": {
			"noble": {
					"stable/rolling": {
						regexp.MustCompile(`my-package-version-.*\.deb`),
					}
			}
	}
}
filePath := "/path/to/files"
ctx := context.TODO()

// Provides access to files
locallyStoredFiles, _ := local.NewFileManager(filePath)

// Executes commands, with optional callbacks to "hooks" for authentication and GPG key access
attuneRunner := exec.NewRunner(/* Add hooks for authentication and GPG key access here */)

// Does the actual publishing
attunePublisher := attune.NewPublisher(attuneRunner)
defer attunePublisher.Close(ctx)

// Tells the publisher about what files need to be published
aptRepoManager := apt.NewManager(locallyStoredFiles, WithRepos(repos), WithPublisher(attunePublisher))
defer aptRepoManager.Close(ctx)

// Alternative: use an errgroup with rate limiting
publishingTasks, _ := aptRepoManager.GetPackagePublishingTasks(ctx)
for publishingTask, _ := range publishingTasks {
  // Run the publishing tasks
	_ := publishingTask(ctx)
}
```
