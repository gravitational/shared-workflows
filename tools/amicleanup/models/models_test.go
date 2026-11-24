package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImage_JSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	want := Image{
		ID:           "ami-0abc",
		Name:         "test-ami",
		Region:       "us-east-1",
		CreationDate: now,
		Public:       true,
		BlockDevices: []BlockDevice{
			{DeviceName: "/dev/xvda", SnapshotID: "snap-1"},
			{DeviceName: "/dev/xvdb", SnapshotID: "snap-2"},
		},
	}

	data, err := json.Marshal(want)
	require.NoError(t, err)

	var got Image
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, want, got)
}

func TestActionResult_JSONRoundTrip(t *testing.T) {
	want := ActionResult{
		ImageID: "ami-0abc",
		Region:  "us-east-1",
		Action:  "deprecate",
		DryRun:  true,
		Err:     "",
	}

	data, err := json.Marshal(want)
	require.NoError(t, err)

	// On the wire, empty Err must be omitted.
	assert.NotContains(t, string(data), "error")

	var got ActionResult
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, want, got)
}

func TestEntryStatus_StableValues(t *testing.T) {
	assert.Equal(t, EntryStatus("pending"), StatusPending)
	assert.Equal(t, EntryStatus("completed"), StatusCompleted)
	assert.Equal(t, EntryStatus("failed"), StatusFailed)
}
