package meta

import (
	"encoding/json"
	"os"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/gravitational/trace"
)

func newFromFile(path string) (*record.Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, trace.Wrap(err, "could not read metadata file")
	}

	var meta record.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, trace.Wrap(err)
	}

	// At the very least we need the primary index ID.
	if meta.ID == "" {
		return nil, trace.BadParameter("missing .id field")
	}

	// Overwrite the record schema used for this producer
	meta.RecordSchemaVersion = record.RecordSchemaVersion
	return &meta, nil

}
