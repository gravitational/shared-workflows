package meta

import (
	"encoding/json"
	"io"
	"os"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/gravitational/trace"
)

func newFromFile(path string) (*record.Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, trace.Wrap(err, "could not read metadata file")
	}
	defer f.Close()
	return newFromReader(f)
}

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
