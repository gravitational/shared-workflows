# ci-normalize

CI utility to normalize test results and capture runner metadata.

## Usage

```sh
# Generate metadata from GH run and write to multiple destinations:
ci-normalize meta \
  --out meta.jsonl \
  --out s3://bucket/meta.jsonl \
  --out - | jq

# Generate junit normalized results:
go run ./cmd junit --meta meta.json  \
  --tests testcase.jsonl \
  --suites suite.jsonl \
  --suites s3://bucket/suite.jsonl \
  junit/*.xml
```
