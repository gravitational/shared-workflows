package meta

import (
	"strings"
	"testing"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newFromReader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string // description of this test case
		jsonMeta string
		wantMeta *record.Meta
		errFn    require.ErrorAssertionFunc
	}{
		{
			name: "all fields",
			jsonMeta: `
{"id":"foobar","record_schema_version":"v1","canonical_meta_schema_version":"v1","provider":"github.com","repository_name":"foo/bar","workflow":"example test workflow","job":"job","run_id":"123123123123","run_attempt":1,"git_sha":"deadbeef","git_ref":"ref","git_base_ref":"base","git_head_ref":"head","actor_name":"foo","actor_id":"9090909090","timestamp":"2026-01-15T13:19:35Z"}
`,
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.NoError(tt, err)
			},
			wantMeta: &record.Meta{
				Common: record.Common{
					ID:                  "foobar",
					RecordSchemaVersion: "v1",
				},
				CanonicalMeta: record.CanonicalMeta{
					CanonicalMetaSchemaVersion: "v1",
					Provider:                   "github.com",
					RepositoryName:             "foo/bar",
					Workflow:                   "example test workflow",
					Job:                        "job",
					RunID:                      "123123123123",
					RunAttempt:                 1,
					SHA:                        "deadbeef",
				},
				GitMeta: record.GitMeta{
					GitRef:  "ref",
					HeadRef: "head",
					BaseRef: "base",
				},
				ActorMeta: record.ActorMeta{
					Actor:   "foo",
					ActorID: "9090909090",
				},
				RunnerMeta: record.RunnerMeta{},
				Timestamp:  "2026-01-15T13:19:35Z",
			},
		},
		{

			name: "hard fail on missing ID",
			jsonMeta: `
{"record_schema_version":"v1","canonical_meta_schema_version":"v1","git_sha":"deadbeef","git_ref":"ref","git_base_ref":"base","git_head_ref":"head","actor_name":"foo","actor_id":"9090909090","timestamp":"2026-01-15T13:19:35Z"}
`,
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.ErrorContains(tt, err, "missing .id field")
			},
		},
		{

			name: "only ID is required",
			jsonMeta: `
{"id":"foobar"}
`,
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.NoError(tt, err)
			},
			wantMeta: &record.Meta{
				Common: record.Common{ID: "foobar", RecordSchemaVersion: "v1"}},
		},
		{

			name: "bad JSON",
			jsonMeta: `
{"id":"foobar"
`,
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.ErrorContains(tt, err, "unexpected EOF")
			},
			wantMeta: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := newFromReader(strings.NewReader(tt.jsonMeta))
			tt.errFn(t, gotErr)
			assert.Equal(t, tt.wantMeta, got)
		})
	}
}
