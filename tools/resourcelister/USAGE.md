# Quick Start Guide

## Prerequisites Check

Before running the tool, verify these prerequisites:

1. **AWS Resource Explorer is enabled**
   ```bash
   aws resource-explorer-2 list-indexes
   ```
   Should return at least one index.

2. **Default view exists**
   ```bash
   aws resource-explorer-2 get-default-view
   ```
   Should return a view ARN.

3. **AWS credentials are configured**
   ```bash
   aws sts get-caller-identity
   ```
   Should return your AWS account details.

## Running the Tool

### Option 1: Using the compiled binary
```bash
./resourcelister > resources.json
```

### Option 2: Using go run
```bash
go run main.go > resources.json
```

### Option 3: With AWS profile
```bash
AWS_PROFILE=myprofile ./resourcelister > resources.json
```

## Viewing Results

### Pretty print with jq
```bash
./resourcelister | jq '.'
```

### Count resources by service
```bash
./resourcelister | jq 'group_by(.service) | map({service: .[0].service, count: length})'
```

### List only EC2 instances
```bash
./resourcelister | jq '.[] | select(.service == "ec2" and .resource_type == "ec2:instance")'
```

### Count resources by region
```bash
./resourcelister | jq 'group_by(.region) | map({region: .[0].region, count: length})'
```

## Troubleshooting

### No resources returned
```bash
# Check if Resource Explorer has indexed resources
aws resource-explorer-2 search --query-string "*"
```

### Permission errors
```bash
# Check IAM permissions
aws iam simulate-principal-policy \
  --policy-source-arn $(aws sts get-caller-identity --query Arn --output text) \
  --action-names resource-explorer-2:ListResources resource-explorer-2:Search
```

### Timeout errors
The tool has a 10-minute timeout. For very large accounts, you may need to increase this in `main.go`:

```go
ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute) // Increase from 10 to 20
```

## Example Workflows

### Export all resources
```bash
./resourcelister > all-resources-$(date +%Y%m%d).json
```

### Find resources without tags
```bash
./resourcelister | jq '.[] | select(.properties | length == 0) | .arn'
```

### Group resources by account (for org aggregator views)
```bash
./resourcelister | jq 'group_by(.owning_account_id) | map({account: .[0].owning_account_id, count: length})'
```

### Export to CSV (requires jq)
```bash
./resourcelister | jq -r '.[] | [.arn, .service, .resource_type, .region] | @csv' > resources.csv
```

## Development

### Run tests
```bash
go test ./... -v
```

### Run tests with coverage
```bash
go test ./... -cover
```

### Build for different platforms
```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o resourcelister-linux-amd64

# macOS
GOOS=darwin GOARCH=amd64 go build -o resourcelister-darwin-amd64

# Windows
GOOS=windows GOARCH=amd64 go build -o resourcelister-windows-amd64.exe
```
