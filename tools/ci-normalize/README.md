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


To use within another workflow:
```yaml
  - name: Configure AWS Credentials
    uses: aws-actions/configure-aws-credentials@a03048d87541d1d9fcf2ecf528a4a65ba9bd7838
    if: always()
    continue-on-error: true
    with:
      aws-region: <region>
      role-to-assume: <role with bucket access>

  - name: Normalize test results and push to s3
    uses: gravitational/shared-workflows/tools/ci-normalize@<SHA>
    if: always()
    continue-on-error: true
    with:
      s3-bucket: "s3://<bucket to use>/ci-metrics"
      junit-files: "$GITHUB_WORKSPACE/test-logs/*.xml"
```