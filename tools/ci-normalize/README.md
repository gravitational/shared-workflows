# ci-normalize

CI utility to normalize test results and capture runner metadata.

## Usage

```sh
# Generate metadata from GH run:
go run ./cmd meta > meta.json

# Generate junit normalized results:
go run ./cmd junit --meta meta.json  --testcase testcase.jsonl --suite suite.jsonl  junit/*.xml 
```
