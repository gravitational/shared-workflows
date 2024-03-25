# AMI cleanup tool

This tool deregisters/deletes Amazon Machine Images (AMIs) for dev tags that are over a month old. Dev tag images are defined as any image matching the name filter `*name*`. This tool runs over all AWS regions enabled in the account.

## Usage:
The local machine must be authenticated with the target AWS account prior to running any of the below commands.

CLI: `ami-cleanup [--dry-run]`
Docker: `docker run --rm --env <AWS_CREDENTIAL_VARS> ghcr.io/gravitational/shared-workflows/ami-cleanup:<version> [--dry-run]`
GHA:
```yaml
- uses: gravitational/shared-workflows/tools/ami-cleanup@<version>
  with:
    dry-run: <true | false>
```

## Building:
This tool requires [Earthly](https://earthly.dev/) to build, used as an alternative to Makefiles. Installation instructions for Earthly are available [here](https://earthly.dev/get-earthly).

To build locally, the following commands are available:
* `earthly +local-binary`
* `earthly +local-tarball`
* `earthly +container-image`
* `earthly +all` - produces all of the above three outputs

To build and cut a release, run `earthly --push --ci +release --GIT_TAG=<tag associated with the release>`. Omit the `--push` arg for a dry run that will not affect production resources.

## Future work
* Make the name filter configurable
* Make the AMI age configurable
* If possible, check if the AMI is in use anywhere prior to deletion
* Add reporting options/table output that lists deleted images
* Mark GH releases as pre-releases if semver shows that they should be
