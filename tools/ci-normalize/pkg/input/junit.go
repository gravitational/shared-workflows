package input

import (
	"context"
	"encoding/xml"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/gravitational/trace"
)

type Option func(*JUnitProducer) error

type JUnitProducer struct {
	file string
	meta record.Common
}

func NewJUnitProducer(file string, opts ...Option) (*JUnitProducer, error) {
	if _, err := os.Stat(file); err != nil {
		return nil, trace.NotFound("junit file %q does not exist: %v", file, err)
	}

	p := &JUnitProducer{file: file}
	for _, opt := range opts {
		if err := opt(p); err != nil {
			return nil, trace.Wrap(err)
		}
	}

	return p, nil
}

func WithMeta(metadata *record.Meta) Option {
	return func(p *JUnitProducer) error {
		p.meta = metadata.Common
		return nil
	}
}

type junitTestSuite struct {
	XMLName xml.Name `xml:"testsuite"`

	Name       string          `xml:"name,attr"`
	Properties []junitProperty `xml:"properties>property"`
	Testcases  []junitTestCase `xml:"testcase"`

	// Fields not used. Included for completeness.
	Tests     int     `xml:"tests,attr"`
	Failures  int     `xml:"failures,attr"`
	Errors    int     `xml:"errors,attr"`
	Skipped   int     `xml:"skipped,attr"`
	Time      float64 `xml:"time,attr"`
	Timestamp string  `xml:"timestamp,attr"`
	Hostname  string  `xml:"hostname,attr"`
	SystemOut string  `xml:"system-out"`
	SystemErr string  `xml:"system-err"`
}

func (ts *junitTestSuite) isValid() bool {
	return ts.Name != "" && len(ts.Testcases) > 0
}

func (ts *junitTestSuite) toRecord(meta record.Common) (*record.Suite, error) {
	t, err := time.Parse(time.RFC3339, ts.Timestamp)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", ts.Timestamp)
		if err != nil {
			return nil, trace.Wrap(err, "failed to parse timestamp")
		}
	}

	return &record.Suite{
		Common:     meta,
		Name:       ts.Name,
		Timestamp:  t.Format(time.RFC3339),
		Tests:      ts.Tests,
		Failures:   ts.Failures,
		Errors:     ts.Errors,
		Skipped:    ts.Skipped,
		DurationMs: int64(ts.Time * 1000),
		Properties: ts.propertiesAsMap(),
	}, nil
}

func (ts *junitTestSuite) propertiesAsMap() map[string]string {
	if len(ts.Properties) == 0 {
		return nil
	}

	m := make(map[string]string, len(ts.Properties))
	for _, p := range ts.Properties {
		m[p.Name] = p.Value
	}
	return m
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

// regex to match ANSI escape sequences
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// SanitizeMessage removes ANSI color codes, non-printable characters, and trims whitespace
func sanitize(msg string) string {
	if msg == "" {
		return ""
	}

	msg = ansiEscape.ReplaceAllString(msg, "")
	msg = strings.ReplaceAll(msg, "\r\n", "\n")
	msg = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 {
			return r
		}
		return -1
	}, msg)

	return strings.TrimSpace(msg)
}

func (jf *junitFailure) safeString() string {
	if jf == nil {
		return ""
	}

	parts := make([]string, 0, 3)

	if jf.Message != "" {
		parts = append(parts, jf.Message)
	}

	if jf.Type != "" {
		parts = append(parts, jf.Type)
	}

	if jf.Text != "" {
		parts = append(parts, jf.Text)
	}

	return sanitize(strings.Join(parts, "\n"))

}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

func (js *junitSkipped) safeString() string {
	if js == nil {
		return ""
	}

	return sanitize(js.Message)
}

type junitTestCase struct {
	Name    string        `xml:"name,attr"`
	Time    float64       `xml:"time,attr"`
	Error   *junitFailure `xml:"error"`
	Failure *junitFailure `xml:"failure"`
	Skipped *junitSkipped `xml:"skipped"`

	// Fields not used. Included for completeness.
	Class     string `xml:"classname,attr"`
	SystemOut string `xml:"system-out"`
	SystemErr string `xml:"system-err"`
}

func (tc *junitTestCase) toRecord(meta record.Common) *record.Testcase {
	return &record.Testcase{
		Common:         meta,
		Name:           tc.Name,
		Classname:      tc.Class,
		DurationMs:     int64(tc.Time * 1000),
		Status:         tc.status(),
		FailureMessage: tc.Failure.safeString(),
		ErrorMessage:   tc.Error.safeString(),
		SkipMessage:    tc.Skipped.safeString(),
	}
}

// safeStatusAndMessage returns the test status and message
func (tc *junitTestCase) status() string {
	switch {
	case tc.Failure != nil:
		return "failed"
	case tc.Error != nil:
		return "error"
	case tc.Skipped != nil:
		return "skipped"
	default:
		return "pass"
	}
}

type junitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

func (p *JUnitProducer) produceFromReader(
	_ context.Context,
	r io.Reader,
	emit func(any) error,
) error {
	decoder := xml.NewDecoder(r)

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return trace.Wrap(err)
		}

		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Local == "testsuite" {
				var ts junitTestSuite
				if err := decoder.DecodeElement(&ts, &el); err != nil {
					return trace.Wrap(err)
				}

				// Skip empty entries
				if !ts.isValid() {
					continue
				}

				suiteInfo, err := ts.toRecord(p.meta)
				if err != nil {
					return trace.Wrap(err)
				}

				if err := emit(suiteInfo); err != nil {
					return trace.Wrap(err)
				}

				for _, tc := range ts.Testcases {
					tcRec := tc.toRecord(p.meta)
					tcRec.SuiteName = ts.Name
					if err := emit(tcRec); err != nil {
						return trace.Wrap(err)
					}
				}
			}

		}
	}

	return nil
}

func (p *JUnitProducer) Produce(ctx context.Context, emit func(any) error) error {
	f, err := os.Open(p.file)
	if err != nil {
		return trace.Wrap(err)
	}
	defer f.Close()
	return p.produceFromReader(ctx, f, emit)
}
