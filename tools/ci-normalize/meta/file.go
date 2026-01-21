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
