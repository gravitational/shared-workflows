package input

import (
	"strings"
	"testing"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJUnitProducer_produceFromReader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string // description of this test case
		xml         string
		meta        record.Common
		wantRecords []any
		errFn       require.ErrorAssertionFunc
		wantErr     bool
	}{
		{
			name: "junit coverage",
			xml: `
<?xml version="1.0" encoding="UTF-8"?>
<testsuite
    name="full-junit-suite"
    tests="4"
    failures="1"
    errors="1"
    skipped="1"
    time="12.345"
    timestamp="2024-01-02T15:04:05Z"
    hostname="ci-runner-01">

    <properties>
        <property name="os" value="linux"/>
        <property name="arch" value="amd64"/>
        <property name="go.version" value="1.22"/>
    </properties>

    <testcase name="test-pass" classname="example.PassTest" time="1.234">
        <system-out>
            normal output
        </system-out>
    </testcase>

    <testcase name="test-failure" classname="example.FailureTest" time="2.345">
        <failure message="assert failed" type="AssertionError">
            <![CDATA[
            \u001b[31mexpected true but got false\u001b[0m
            at example.FailureTest:42
            ]]>
        </failure>
        <system-err>stderr output</system-err>
    </testcase>

    <testcase name="test-error" classname="example.ErrorTest" time="3.456">
        <error message="panic occurred" type="RuntimeError">
            <![CDATA[
            panic: index out of range
            at example.ErrorTest:99
            ]]>
        </error>
    </testcase>

    <testcase name="test-skipped" classname="example.SkippedTest" time="0.0">
        <skipped message="feature not implemented yet"/>
    </testcase>

    <system-out>
        suite level stdout
    </system-out>

    <system-err>
        suite level stderr
    </system-err>
</testsuite>
`,
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.NoError(tt, err, i...)
			},
			wantRecords: []any{
				&record.Suite{
					Name:       "full-junit-suite",
					Timestamp:  "2024-01-02T15:04:05Z",
					Tests:      4,
					Failures:   1,
					Errors:     1,
					Skipped:    1,
					DurationMs: 12345,
					Properties: map[string]string{
						"arch":       "amd64",
						"go.version": "1.22",
						"os":         "linux",
					},
				},
				&record.Testcase{
					Name:       "test-pass",
					SuiteName:  "full-junit-suite",
					Classname:  "example.PassTest",
					DurationMs: 1234,
					Status:     "pass",
				},
				&record.Testcase{
					Name:           "test-failure",
					SuiteName:      "full-junit-suite",
					Classname:      "example.FailureTest",
					DurationMs:     2345,
					Status:         "failed",
					FailureMessage: "assert failed\nAssertionError\n\n            \n            \\u001b[31mexpected true but got false\\u001b[0m\n            at example.FailureTest:42",
				},
				&record.Testcase{

					Name:         "test-error",
					SuiteName:    "full-junit-suite",
					Classname:    "example.ErrorTest",
					DurationMs:   3456,
					Status:       "error",
					SkipMessage:  "",
					ErrorMessage: "panic occurred\nRuntimeError\n\n            \n            panic: index out of range\n            at example.ErrorTest:99",
				},
				&record.Testcase{
					Name:        "test-skipped",
					SuiteName:   "full-junit-suite",
					Classname:   "example.SkippedTest",
					Status:      "skipped",
					SkipMessage: "feature not implemented yet",
				},
			},
		},
		{
			name: "meta propagated",
			xml: `
<?xml version="1.0" encoding="UTF-8"?>
<testsuite
    name="full-junit-suite"
    tests="4"
    failures="1"
    errors="1"
    skipped="1"
    time="12.345"
    timestamp="2024-01-02T15:04:05Z"
    hostname="ci-runner-01">
    <testcase name="tc1" classname="example.PassTest" time="1.234"></testcase>
</testsuite>
`,
			meta: record.Common{
				ID:                  "deadbeef",
				RecordSchemaVersion: "v13",
			},
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.NoError(tt, err, i...)
			},
			wantRecords: []any{
				&record.Suite{
					Name:       "full-junit-suite",
					Timestamp:  "2024-01-02T15:04:05Z",
					Tests:      4,
					Failures:   1,
					Errors:     1,
					Skipped:    1,
					DurationMs: 12345,
					Common: record.Common{
						ID:                  "deadbeef",
						RecordSchemaVersion: "v13",
					},
				},
				&record.Testcase{
					Name:       "tc1",
					SuiteName:  "full-junit-suite",
					Classname:  "example.PassTest",
					DurationMs: 1234,
					Status:     "pass",
					Common: record.Common{
						ID:                  "deadbeef",
						RecordSchemaVersion: "v13",
					},
				},
			},
		},
		{
			name: "truncated input",
			xml: `
<testsuite name="suite">
<testcase name="test1">
`,
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.Error(tt, err)
				assert.ErrorContains(tt, err, "XML syntax error")
			},
		},

		{
			name: "mismatched tags",
			xml: `
<testsuite name="suite">
<testcase name="test1"></testsuite>
`,
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.Error(tt, err)
				assert.ErrorContains(tt, err, "XML syntax error")
			},
		},
		{
			name: "bad suite timestamp",
			xml: `
<testsuite name="full-junit-suite" timestamp="ooops">
	<testcase name="test1"></testcase>
</testsuite>
`,
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.Error(tt, err)
				assert.ErrorContains(tt, err, "failed to parse timestamp")
			},
		},
		{
			name: "empty suites are skipped",
			xml: `
<testsuite name="full-junit-suite" timestamp="ooops">
</testsuite>
`,
			errFn: func(tt require.TestingT, err error, i ...interface{}) {
				assert.NoError(tt, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &JUnitProducer{meta: tt.meta}
			var emitted []any
			emit := func(v any) error {
				emitted = append(emitted, v)
				return nil
			}
			gotErr := p.produceFromReader(t.Context(), strings.NewReader(tt.xml), emit)
			tt.errFn(t, gotErr)
			assert.Equal(t, tt.wantRecords, emitted)
		})
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "only whitespace trimmed",
			in:   "   hello world   ",
			want: "hello world",
		},
		{
			name: "ansi escape sequences removed",
			in:   "\x1b[31mred text\x1b[0m",
			want: "red text",
		},
		{
			name: "multiple ansi sequences removed",
			in:   "\x1b[31mred\x1b[0m and \x1b[32mgreen\x1b[0m",
			want: "red and green",
		},
		{
			name: "windows newlines normalized",
			in:   "line1\r\nline2\r\nline3",
			want: "line1\nline2\nline3",
		},
		{
			name: "non printable characters removed",
			in:   "a\x00b\x01c\x02d",
			want: "abcd",
		},
		{
			name: "tab and newline preserved",
			in:   "line1\n\tline2",
			want: "line1\n\tline2",
		},
		{
			name: "mixed ansi, control chars, whitespace, trailing whitespace removed",
			in:   " \x1b[31mFAIL\x1b[0m\r\n\x07reason\t\n ",
			want: "FAIL\nreason",
		},
		{
			name: "carriage return without newline removed",
			in:   "hello\rworld",
			want: "helloworld",
		},
		{
			name: "unicode characters preserved",
			in:   "✓ passed – 測試",
			want: "✓ passed – 測試",
		},
		{
			name: "leading and trailing newlines trimmed",
			in:   "\n\nmessage\n\n",
			want: "message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitize(tt.in))
		})
	}
}
