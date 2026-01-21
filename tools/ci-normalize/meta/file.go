package meta

import (
	"encoding/json"
	"io"
	"os"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
	"github.com/gravitational/trace"
)

// newFromFile reads metadata from the specified JSON file.
func newFromFile(path string) (*record.Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, trace.Wrap(err, "could not read metadata file")
	}
	defer func() {
		_ = f.Close()
	}()
	return newFromReader(f)
}

// newFromReader reads metadata from the provided reader in JSON format.
func newFromReader(r io.Reader) (*record.Meta, error) {
	var meta record.Meta

	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&meta); err != nil {
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
