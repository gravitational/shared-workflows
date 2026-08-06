package models

import (
	"encoding/json"
	"time"
)

// Resource represents an AWS resource discovered by Resource Explorer
type Resource struct {
	ARN             string             `json:"arn"`
	OwningAccountID string             `json:"owning_account_id"`
	Region          string             `json:"region"`
	ResourceType    string             `json:"resource_type"`
	Service         string             `json:"service"`
	LastReportedAt  time.Time          `json:"last_reported_at"`
	Properties      []ResourceProperty `json:"properties,omitempty"`
}

// ResourceProperty represents a property of an AWS resource
type ResourceProperty struct {
	Name           string          `json:"name"`
	Data           json.RawMessage `json:"data"`
	LastReportedAt time.Time       `json:"last_reported_at"`
}
