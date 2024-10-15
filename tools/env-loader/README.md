# env-loader

This tool loads "environment values" from configuration and secret sources, and
outputs them in various formats. This can be used as a CLI tool, or as a Go
module.

The following input formats are currently supported:
* YAML
* JSON
* SOPS

The following output formats are currently supported:
* dotenv

When loading environment values from a given "value set", the tool will search
for and load the following files in order:
* `<root path>/common.*` (repo level, common to everything)
* `<root path>/<env name>/common.*` (environment level, common to all values in
  an environment)
* `<root path>/<env name>/<value set name>.*` (value set, specifically
  requested values)

The root path is typically the `environments` directory directly under the git
repo root.

If there is a duplicate key found, the last loaded value (from the file lowest
down on the list) will be loaded.

## Terminology

Environment value - a configuration or secret key/value pair. These can come
from different sources, and may or may not be loaded into a process's
environment.

Value set - A logical grouping of value sets, as well as a set of common
environment values. Typically secrets in this grouping will share a single set
of permissions for access.

Environment - A logical grouping of value sets, as well as a set of common
environment values.

## Security

This tool does not _directly_ implement encryption for secrets, however, some
input formats (such as SOPS) may call other modules or libraries to decrypt
files. Access to secret key material must be controlled outside of this tool.

### SOPS

SOPS files can use any key provider supported by SOPS (age, PGP, AWS KMS, GCP
KMS, etc.). This tool will obey SOPS configuration (.sops.yaml file, environment
variables) when attempting to load SOPS files. Callers of this module or CLI
must ensure that access to secret material is already setup, typically via
environment variables.

A common usage pattern with Cloud providers with CI/CD pipelines is to store the
name of Cloud roles with permission to decrypt secrets in the CI/CD tool value
store, and then authenticate with the Cloud provider via OIDC. This puts the
root of trust entirely on the CI/CD tool OIDC provider, rather than the value
store.

## Future work

* Add tests
* Add dev tooling
* Add CI/CD pipelines
* Add GitHub action
