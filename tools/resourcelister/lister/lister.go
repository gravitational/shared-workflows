package lister

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourceexplorer2"
	"github.com/aws/aws-sdk-go-v2/service/resourceexplorer2/types"
	"github.com/shared-workflows/tools/resourcelister/models"
)

// ResourceExplorerClient defines the interface for AWS Resource Explorer operations
type ResourceExplorerClient interface {
	ListResources(ctx context.Context, params *resourceexplorer2.ListResourcesInput, optFns ...func(*resourceexplorer2.Options)) (*resourceexplorer2.ListResourcesOutput, error)
}

// Lister handles listing AWS resources
type Lister struct {
	client ResourceExplorerClient
}

// NewLister creates a new Lister instance
func NewLister(client ResourceExplorerClient) *Lister {
	return &Lister{
		client: client,
	}
}

// ListAllResources retrieves all AWS resources using pagination
func (l *Lister) ListAllResources(ctx context.Context) ([]models.Resource, error) {
	var allResources []models.Resource
	var nextToken *string

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		input := &resourceexplorer2.ListResourcesInput{
			MaxResults: aws.Int32(1000),
			NextToken:  nextToken,
		}

		output, err := l.client.ListResources(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list resources: %w", err)
		}

		for _, resource := range output.Resources {
			allResources = append(allResources, convertResource(resource))
		}

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return allResources, nil
}

// convertResource converts AWS SDK resource type to our model
func convertResource(r types.Resource) models.Resource {
	resource := models.Resource{
		Properties: []models.ResourceProperty{},
	}

	if r.Arn != nil {
		resource.ARN = *r.Arn
	}
	if r.OwningAccountId != nil {
		resource.OwningAccountID = *r.OwningAccountId
	}
	if r.Region != nil {
		resource.Region = *r.Region
	}
	if r.ResourceType != nil {
		resource.ResourceType = *r.ResourceType
	}
	if r.Service != nil {
		resource.Service = *r.Service
	}
	if r.LastReportedAt != nil {
		resource.LastReportedAt = *r.LastReportedAt
	}

	for _, prop := range r.Properties {
		converted := convertResourceProperty(prop)
		if converted != nil {
			resource.Properties = append(resource.Properties, *converted)
		}
	}

	return resource
}

// convertResourceProperty converts AWS SDK property type to our model
func convertResourceProperty(p types.ResourceProperty) *models.ResourceProperty {
	if p.Name == nil {
		return nil
	}

	prop := models.ResourceProperty{
		Name: *p.Name,
	}

	if p.LastReportedAt != nil {
		prop.LastReportedAt = *p.LastReportedAt
	}

	if p.Data != nil {
		data, err := marshalPropertyData(p.Data)
		if err == nil {
			prop.Data = data
		}
	}

	return &prop
}

// marshalPropertyData converts the property data to JSON
func marshalPropertyData(data interface{}) (json.RawMessage, error) {
	if data == nil {
		return json.RawMessage("null"), nil
	}

	// The data is a document.Interface, so we need to unmarshal it first
	var v interface{}
	if unmarshaler, ok := data.(interface{ UnmarshalSmithyDocument(interface{}) error }); ok {
		if err := unmarshaler.UnmarshalSmithyDocument(&v); err != nil {
			return nil, fmt.Errorf("failed to unmarshal document: %w", err)
		}
		return json.Marshal(v)
	}

	// Fallback to direct marshaling
	return json.Marshal(data)
}
