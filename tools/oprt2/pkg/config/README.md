# config

This package is intended to parse and validate configuration files. Structures
within this package should map 1:1 with configuration fields.

Example config:
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
