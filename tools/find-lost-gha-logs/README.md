# find-lost-gha-logs

This tool collects a list of workflow jobs that GitHub is unable to produce logs
for. The intended use of this tool is to make collecting a list of failing jobs
for GitHub support, so that we can claim SLA service credits and push then to
fix their infrastructure.

## Usage

```console
$ find-lost-gha-logs --help
Usage of find-lost-gha-logs:
  -days-to-check int
        Including the current date, the number of days to look through for workflow jobs (default 90)
  -include-self-hosted
        True to include workflow runs without logs for self-hosted runner jobs, false otherwise
  -org string
        GitHub organization or owner to check against (default "gravitational")
  -repo string
        GitHub repo to check against (default "teleport.e")
  -token string
        GitHub token to use (will default to ${GITHUB_TOKEN} env var if unset) (default "${GITHUB_TOKEN}")
```

## Example output

[See here](https://support.github.com/ticket/enterprise/9266/3846321#tc-42441438499348).
