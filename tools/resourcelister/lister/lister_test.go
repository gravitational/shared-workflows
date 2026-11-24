package lister

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/resourceexplorer2"
	"github.com/aws/aws-sdk-go-v2/service/resourceexplorer2/document"
	"github.com/aws/aws-sdk-go-v2/service/resourceexplorer2/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/shared-workflows/tools/resourcelister/models"
)

// mockResourceExplorerClient implements the ResourceExplorerClient interface for testing
type mockResourceExplorerClient struct {
	pages     []resourceexplorer2.ListResourcesOutput
	pageIndex int
	err       error
}

func (m *mockResourceExplorerClient) ListResources(ctx context.Context, params *resourceexplorer2.ListResourcesInput, optFns ...func(*resourceexplorer2.Options)) (*resourceexplorer2.ListResourcesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}

	if m.pageIndex >= len(m.pages) {
		return &resourceexplorer2.ListResourcesOutput{
			Resources: []types.Resource{},
		}, nil
	}

	page := m.pages[m.pageIndex]
	m.pageIndex++
	return &page, nil
}

func TestListAllResources_EmptyResult(t *testing.T) {
	client := &mockResourceExplorerClient{
		pages: []resourceexplorer2.ListResourcesOutput{
			{
				Resources: []types.Resource{},
			},
		},
	}

	lister := NewLister(client)
	resources, err := lister.ListAllResources(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
	}
}

func TestListAllResources_SinglePage(t *testing.T) {
	now := time.Now()
	client := &mockResourceExplorerClient{
		pages: []resourceexplorer2.ListResourcesOutput{
			{
				Resources: []types.Resource{
					{
						Arn:             ptr.String("arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0"),
						OwningAccountId: ptr.String("123456789012"),
						Region:          ptr.String("us-east-1"),
						ResourceType:    ptr.String("ec2:instance"),
						Service:         ptr.String("ec2"),
						LastReportedAt:  &now,
					},
					{
						Arn:             ptr.String("arn:aws:s3:::my-bucket"),
						OwningAccountId: ptr.String("123456789012"),
						Region:          ptr.String("us-east-1"),
						ResourceType:    ptr.String("s3:bucket"),
						Service:         ptr.String("s3"),
						LastReportedAt:  &now,
					},
				},
			},
		},
	}

	lister := NewLister(client)
	resources, err := lister.ListAllResources(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(resources))
	}

	if resources[0].ARN != "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0" {
		t.Errorf("unexpected ARN: %s", resources[0].ARN)
	}

	if resources[0].Service != "ec2" {
		t.Errorf("expected service 'ec2', got %s", resources[0].Service)
	}

	if resources[1].Service != "s3" {
		t.Errorf("expected service 's3', got %s", resources[1].Service)
	}
}

func TestListAllResources_MultiplePages(t *testing.T) {
	now := time.Now()
	client := &mockResourceExplorerClient{
		pages: []resourceexplorer2.ListResourcesOutput{
			{
				Resources: []types.Resource{
					{
						Arn:             ptr.String("arn:aws:ec2:us-east-1:123456789012:instance/i-1"),
						OwningAccountId: ptr.String("123456789012"),
						Region:          ptr.String("us-east-1"),
						ResourceType:    ptr.String("ec2:instance"),
						Service:         ptr.String("ec2"),
						LastReportedAt:  &now,
					},
				},
				NextToken: ptr.String("token1"),
			},
			{
				Resources: []types.Resource{
					{
						Arn:             ptr.String("arn:aws:ec2:us-east-1:123456789012:instance/i-2"),
						OwningAccountId: ptr.String("123456789012"),
						Region:          ptr.String("us-east-1"),
						ResourceType:    ptr.String("ec2:instance"),
						Service:         ptr.String("ec2"),
						LastReportedAt:  &now,
					},
				},
				NextToken: ptr.String("token2"),
			},
			{
				Resources: []types.Resource{
					{
						Arn:             ptr.String("arn:aws:ec2:us-east-1:123456789012:instance/i-3"),
						OwningAccountId: ptr.String("123456789012"),
						Region:          ptr.String("us-east-1"),
						ResourceType:    ptr.String("ec2:instance"),
						Service:         ptr.String("ec2"),
						LastReportedAt:  &now,
					},
				},
			},
		},
	}

	lister := NewLister(client)
	resources, err := lister.ListAllResources(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resources) != 3 {
		t.Errorf("expected 3 resources, got %d", len(resources))
	}
}

func TestListAllResources_WithProperties(t *testing.T) {
	now := time.Now()
	client := &mockResourceExplorerClient{
		pages: []resourceexplorer2.ListResourcesOutput{
			{
				Resources: []types.Resource{
					{
						Arn:             ptr.String("arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0"),
						OwningAccountId: ptr.String("123456789012"),
						Region:          ptr.String("us-east-1"),
						ResourceType:    ptr.String("ec2:instance"),
						Service:         ptr.String("ec2"),
						LastReportedAt:  &now,
						Properties: []types.ResourceProperty{
							{
								Name:           ptr.String("InstanceType"),
								Data:           document.NewLazyDocument("t2.micro"),
								LastReportedAt: &now,
							},
						},
					},
				},
			},
		},
	}

	lister := NewLister(client)
	resources, err := lister.ListAllResources(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}

	if len(resources[0].Properties) != 1 {
		t.Errorf("expected 1 property, got %d", len(resources[0].Properties))
	}

	if resources[0].Properties[0].Name != "InstanceType" {
		t.Errorf("expected property name 'InstanceType', got %s", resources[0].Properties[0].Name)
	}
}

func TestListAllResources_APIError(t *testing.T) {
	expectedErr := errors.New("API error")
	client := &mockResourceExplorerClient{
		err: expectedErr,
	}

	lister := NewLister(client)
	_, err := lister.ListAllResources(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestListAllResources_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &mockResourceExplorerClient{
		pages: []resourceexplorer2.ListResourcesOutput{
			{
				Resources: []types.Resource{},
			},
		},
	}

	lister := NewLister(client)
	_, err := lister.ListAllResources(ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestConvertResource(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    types.Resource
		expected models.Resource
	}{
		{
			name: "complete resource",
			input: types.Resource{
				Arn:             ptr.String("arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0"),
				OwningAccountId: ptr.String("123456789012"),
				Region:          ptr.String("us-east-1"),
				ResourceType:    ptr.String("ec2:instance"),
				Service:         ptr.String("ec2"),
				LastReportedAt:  &now,
			},
			expected: models.Resource{
				ARN:             "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0",
				OwningAccountID: "123456789012",
				Region:          "us-east-1",
				ResourceType:    "ec2:instance",
				Service:         "ec2",
				LastReportedAt:  now,
				Properties:      []models.ResourceProperty{},
			},
		},
		{
			name: "resource with nil fields",
			input: types.Resource{
				Arn: ptr.String("arn:aws:s3:::my-bucket"),
			},
			expected: models.Resource{
				ARN:             "arn:aws:s3:::my-bucket",
				OwningAccountID: "",
				Region:          "",
				ResourceType:    "",
				Service:         "",
				LastReportedAt:  time.Time{},
				Properties:      []models.ResourceProperty{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertResource(tt.input)

			if result.ARN != tt.expected.ARN {
				t.Errorf("ARN mismatch: got %v, want %v", result.ARN, tt.expected.ARN)
			}
			if result.Service != tt.expected.Service {
				t.Errorf("Service mismatch: got %v, want %v", result.Service, tt.expected.Service)
			}
		})
	}
}
