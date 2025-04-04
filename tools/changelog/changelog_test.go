/*
Copyright 2024 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeTestData(t *testing.T, data []byte) []github.ChangelogPR {
	prs := []github.ChangelogPR{}
	dec := json.NewDecoder(bytes.NewReader(data))
	require.NoError(t, dec.Decode(&prs))
	return prs
}

func TestToChangelog(t *testing.T) {
	testCases := []struct {
		name           string
		expectedFile   string
		excludePRLinks bool
	}{
		{
			name:           "include-links",
			expectedFile:   "expected-cl.md",
			excludePRLinks: false,
		},
		{
			name:           "exclude-links",
			expectedFile:   "expected-cl-no-links.md",
			excludePRLinks: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			prsText, err := os.ReadFile(filepath.Join("testdata", "listed-prs.json"))
			require.NoError(t, err)
			expectedCL, err := os.ReadFile(filepath.Join("testdata", tt.expectedFile))
			require.NoError(t, err)

			prs := decodeTestData(t, prsText)

			gen := &changelogGenerator{excludePRLinks: tt.excludePRLinks}
			got, err := gen.toChangelog(prs)
			assert.NoError(t, err)
			assert.Equal(t, string(expectedCL), got)
		})
	}
}
