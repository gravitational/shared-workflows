package meta

import (
	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
	"github.com/gravitational/trace"
)

// New creates a new metadata record.
// If a metaFile is provided, it is used as the source of metadata.
// Otherwise, metadata is read from GitHub environment variables.
func New(metaFile *string) (*record.Meta, error) {
	var meta *record.Meta

	switch {
	case metaFile != nil && *metaFile != "":
		if m, err := newFromFile(*metaFile); err != nil {
			return nil, trace.Wrap(err, "reading from file: %q", *metaFile)
		} else if m != nil && m.ID != "" {
			meta = m
			break
		}
		fallthrough
	default:
		if m, err := newFromGithubEnv(); err != nil {
			return nil, trace.Wrap(err)
		} else if m != nil && m.ID != "" {
			meta = m
		}
	}

	return meta, nil
}
