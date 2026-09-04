package models

import "time"

// Image is the tool's view of an AMI. It captures only the fields amicleanup needs.
type Image struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Region       string        `json:"region"`
	CreationDate time.Time     `json:"creation_date"`
	Public       bool          `json:"public"`
	BlockDevices []BlockDevice `json:"block_devices,omitempty"`
}

// BlockDevice captures a single EBS-backed block device on an AMI.
// SnapshotID is the EBS snapshot the device was created from; it is the snapshot
// that --action=delete will also delete after deregistering the AMI.
type BlockDevice struct {
	DeviceName string `json:"device_name"`
	SnapshotID string `json:"snapshot_id"`
}

// ActionResult is the outcome of applying a single action to a single AMI
// (or to a single snapshot that is part of a delete action).
type ActionResult struct {
	ImageID string `json:"image_id"`
	Region  string `json:"region"`
	Action  string `json:"action"`
	DryRun  bool   `json:"dry_run"`
	Err     string `json:"error,omitempty"`
}

// EntryStatus is the lifecycle of a plan entry.
type EntryStatus string

const (
	StatusPending   EntryStatus = "pending"
	StatusCompleted EntryStatus = "completed"
	StatusFailed    EntryStatus = "failed"
)
