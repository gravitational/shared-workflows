// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
