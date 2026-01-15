# ci-normalize

CI utility to normalize test results and capture runner metadata.

## Usage

```sh
# Generate metadata from GH run and write to multiple destinations:
ci-normalize meta \
  --meta meta.jsonl \
  --meta s3://bucket/meta.jsonl \
  --meta - | jq

# Generate junit normalized results from existing metadata file:
ci-normalize junit --from-meta meta.json  \
  --tests testcase.jsonl \
  --suites suite.jsonl \
  --suites s3://bucket/suite.jsonl \
  --meta /dev/null \
  junit/*.xml
```
