# ci-normalize

CI utility to normalize test results and capture runner metadata.

## Usage

### cli

```sh
# Generate metadata from GH run and write to multiple destinations:
ci-normalize meta \
  --meta meta.jsonl \
  --meta s3://bucket/meta.jsonl \
  --meta - | jq

# Generate normalized results from junit xml:
ci-normalize junit --from-meta meta.json  \
  --tests tests.jsonl \
  --suites suites.jsonl \
  --suites s3://bucket/suites.jsonl \
  --meta /dev/null \
  junit/*.xml
```

### Github Action

To use within another workflow:
```yaml
  - name: User test step

  - name: Configure AWS Credentials
    uses: aws-actions/configure-aws-credentials@a03048d87541d1d9fcf2ecf528a4a65ba9bd7838
    id: aws-setup
    if: always()
    with:
      aws-region: <region>
      role-to-assume: <role with bucket access>

  - name: Normalize test results and push to s3
    uses: gravitational/shared-workflows/tools/ci-normalize@<SHA>
    if: always() && steps.aws-setup.outcome == 'success'
    continue-on-error: true
    with:
      s3-bucket: "s3://<bucket to use>/ci-metrics"
      junit-files: "$GITHUB_WORKSPACE/test-logs/*.xml"
```

## Supported Formats

Inputs:
- [x] JUnit XML

Output:
- [x] jsonl

The output file schema can be found in `pkg/record/record.go`.
