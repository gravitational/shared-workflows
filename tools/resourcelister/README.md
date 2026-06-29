# AWS Resource Lister

A standalone Go program that lists ALL AWS resources in an account using AWS Resource Explorer 2's ListResources API.

## Features

- Lists all AWS resources across all supported services (200+ resource types)
- Handles pagination automatically
- Built-in rate limit handling via AWS SDK retry logic
- Outputs clean JSON to stdout
- Captures resource properties and metadata

## Prerequisites

### AWS Resource Explorer Setup

AWS Resource Explorer 2 must be enabled and configured in your AWS account:

1. **Enable Resource Explorer**: Navigate to the Resource Explorer console
2. **Create an Index**: Create at least one index (aggregator or local)
3. **Configure a Default View**: Set up a default view for querying resources

For more information, see the [AWS Resource Explorer User Guide](https://docs.aws.amazon.com/resource-explorer/latest/userguide/getting-started.html).

### IAM Permissions

The AWS credentials used must have the following IAM permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "resource-explorer-2:Search",
        "resource-explorer-2:ListResources"
      ],
      "Resource": "*"
    }
  ]
}
```

### AWS Credentials

The program uses the default AWS credential chain:
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
2. Shared credentials file (`~/.aws/credentials`)
3. IAM role (if running on EC2/ECS/Lambda)

You can also specify a profile using the `AWS_PROFILE` environment variable.

## Installation

### From Source

```bash
go build -o resourcelister
```

## Usage

### Basic Usage

```bash
# List all resources and save to file
./resourcelister > resources.json

# Or run directly with go
go run main.go > resources.json
```

### With AWS Profile

```bash
AWS_PROFILE=production ./resourcelister > prod-resources.json
```

### With Specific Region

```bash
AWS_REGION=us-east-1 ./resourcelister > resources.json
```

## Output Format

The program outputs a JSON array of resources with the following structure (see `example-output.json` for a complete example):

```json
[
  {
    "arn": "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0",
    "owning_account_id": "123456789012",
    "region": "us-east-1",
    "resource_type": "ec2:instance",
    "service": "ec2",
    "last_reported_at": "2024-01-01T12:00:00Z",
    "properties": [
      {
        "name": "InstanceType",
        "data": "t2.micro",
        "last_reported_at": "2024-01-01T12:00:00Z"
      }
    ]
  }
]
```

### Fields

- `arn`: Amazon Resource Name uniquely identifying the resource
- `owning_account_id`: AWS account ID that owns the resource
- `region`: AWS region where the resource exists
- `resource_type`: Type of resource (e.g., "ec2:instance", "s3:bucket")
- `service`: AWS service that owns the resource
- `last_reported_at`: Timestamp when Resource Explorer last indexed this resource
- `properties`: Array of additional resource properties (varies by resource type)

## Performance

- **Timeout**: 10 minutes maximum (configurable in `main.go`)
- **Pagination**: Fetches up to 1000 resources per API call
- **Typical times**:
  - ~1,000 resources: 10-30 seconds
  - ~10,000 resources: 2-5 minutes

## Error Handling

The program exits with the following codes:

- `0`: Success
- `1`: AWS API error, configuration error, or output error

Common errors:

- **No credentials found**: Set AWS credentials via environment variables or AWS profile
- **AccessDeniedException**: Ensure IAM permissions are configured
- **ResourceNotFoundException**: Resource Explorer is not enabled or no default view exists
- **UnauthorizedException**: No default view configured in Resource Explorer

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run specific package tests
go test ./lister/ -v
go test ./models/ -v
```

### Project Structure

```
resourcelister/
├── main.go                 # Application entry point
├── go.mod                  # Go module dependencies
├── go.sum                  # Dependency checksums
├── lister/
│   ├── lister.go          # Core resource listing logic
│   └── lister_test.go     # Lister unit tests
├── models/
│   ├── resource.go        # Data models for resources
│   └── resource_test.go   # Model unit tests
└── README.md              # This file
```

## Supported Resource Types

AWS Resource Explorer supports 200+ resource types across AWS services. For the complete list, see [Supported Resource Types](https://docs.aws.amazon.com/resource-explorer/latest/userguide/supported-resource-types.html).

Examples include:
- EC2 instances, volumes, security groups
- S3 buckets
- Lambda functions
- RDS databases
- IAM roles and policies
- VPCs and subnets
- And many more...

## Limitations

- Resources are collected in memory (suitable for most AWS accounts)
- Requires Resource Explorer to be enabled and configured
- Only lists resources indexed by Resource Explorer (new resources may have slight delay)
- Region scope depends on Resource Explorer index configuration (aggregator vs local)

## Future Enhancements

Potential improvements (not currently implemented):

- Filter support via CLI flags (by service, region, tags)
- Multiple output formats (CSV, table)
- Streaming JSON output for very large result sets
- Progress indicators to stderr
- Incremental updates to track resource changes

## Troubleshooting

### "Resource Explorer not found"

Enable Resource Explorer in your AWS account and create at least one index.

### "No default view"

Configure a default view in Resource Explorer or specify a ViewArn in the code.

### "Access Denied"

Verify IAM permissions include `resource-explorer-2:ListResources` and `resource-explorer-2:Search`.

### "No resources found"

- Ensure Resource Explorer has had time to index resources (can take several minutes initially)
- Verify the index type (aggregator indexes show resources from all regions)
- Check if resources exist in the indexed regions

## References

- [AWS Resource Explorer API Reference](https://docs.aws.amazon.com/resource-explorer/latest/apireference/)
- [ListResources API](https://docs.aws.amazon.com/resource-explorer/latest/apireference/API_ListResources.html)
- [AWS SDK for Go v2](https://aws.github.io/aws-sdk-go-v2/)
- [Supported Resource Types](https://docs.aws.amazon.com/resource-explorer/latest/userguide/supported-resource-types.html)

## License

This project is provided as-is for use within the organization.
