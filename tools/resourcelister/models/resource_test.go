package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestResourceJSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		resource Resource
		wantErr  bool
	}{
		{
			name: "complete resource",
			resource: Resource{
				ARN:             "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0",
				OwningAccountID: "123456789012",
				Region:          "us-east-1",
				ResourceType:    "ec2:instance",
				Service:         "ec2",
				LastReportedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Properties: []ResourceProperty{
					{
						Name:           "InstanceType",
						Data:           json.RawMessage(`"t2.micro"`),
						LastReportedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "resource without properties",
			resource: Resource{
				ARN:             "arn:aws:s3:::my-bucket",
				OwningAccountID: "123456789012",
				Region:          "us-east-1",
				ResourceType:    "s3:bucket",
				Service:         "s3",
				LastReportedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Properties:      nil,
			},
			wantErr: false,
		},
		{
			name: "resource with empty properties",
			resource: Resource{
				ARN:             "arn:aws:lambda:us-west-2:123456789012:function:my-function",
				OwningAccountID: "123456789012",
				Region:          "us-west-2",
				ResourceType:    "lambda:function",
				Service:         "lambda",
				LastReportedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Properties:      []ResourceProperty{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resource)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				var unmarshaled Resource
				if err := json.Unmarshal(data, &unmarshaled); err != nil {
					t.Errorf("Unmarshal() error = %v", err)
					return
				}

				if unmarshaled.ARN != tt.resource.ARN {
					t.Errorf("ARN mismatch: got %v, want %v", unmarshaled.ARN, tt.resource.ARN)
				}
				if unmarshaled.Service != tt.resource.Service {
					t.Errorf("Service mismatch: got %v, want %v", unmarshaled.Service, tt.resource.Service)
				}
			}
		})
	}
}

func TestResourcePropertyJSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		prop     ResourceProperty
		wantErr  bool
	}{
		{
			name: "string property",
			prop: ResourceProperty{
				Name:           "State",
				Data:           json.RawMessage(`"running"`),
				LastReportedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name: "object property",
			prop: ResourceProperty{
				Name:           "Tags",
				Data:           json.RawMessage(`{"Environment":"production","Owner":"team"}`),
				LastReportedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name: "array property",
			prop: ResourceProperty{
				Name:           "SecurityGroups",
				Data:           json.RawMessage(`["sg-123","sg-456"]`),
				LastReportedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.prop)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				var unmarshaled ResourceProperty
				if err := json.Unmarshal(data, &unmarshaled); err != nil {
					t.Errorf("Unmarshal() error = %v", err)
					return
				}

				if unmarshaled.Name != tt.prop.Name {
					t.Errorf("Name mismatch: got %v, want %v", unmarshaled.Name, tt.prop.Name)
				}
			}
		})
	}
}

func TestResourceListJSONMarshaling(t *testing.T) {
	resources := []Resource{
		{
			ARN:             "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0",
			OwningAccountID: "123456789012",
			Region:          "us-east-1",
			ResourceType:    "ec2:instance",
			Service:         "ec2",
			LastReportedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			ARN:             "arn:aws:s3:::my-bucket",
			OwningAccountID: "123456789012",
			Region:          "us-east-1",
			ResourceType:    "s3:bucket",
			Service:         "s3",
			LastReportedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}

	data, err := json.Marshal(resources)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var unmarshaled []Resource
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(unmarshaled) != len(resources) {
		t.Errorf("Length mismatch: got %d, want %d", len(unmarshaled), len(resources))
	}
}
