package input

import (
	"context"
	"encoding/json"
	"os"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/gravitational/trace"
)

type Producer interface {
	Produce(ctx context.Context, emit func(any) error) error
}

func ReadMetaFile(path string) (*record.Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var meta record.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, trace.Wrap(err)
	}

	// At the very least we need the primary index ID.
	if meta.Common.ID == "" {
		return nil, trace.BadParameter("missing .id field")
	}

	// Overwrite the record schema used for this producer
	meta.Common.RecordSchemaVersion = meta.RecordSchemaVersion
	return &meta, nil
}
