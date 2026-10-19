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

package reporter

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

var _ Reporter = (*Stdout)(nil)

const TypeStdout = "stdout"

// Stdout renders a document as plain text.
type Stdout struct {
	name string
	out  io.Writer
	// maxRows caps the rows printed per table. <=0 means no limit.
	maxRows int
}

// NewStdout creates a new instance of [Stdout].
func NewStdout(name string, out io.Writer, maxRows int) *Stdout {
	return &Stdout{name: name, out: out, maxRows: maxRows}
}

// Name returns the configured name of this reporter.
func (s *Stdout) Name() string {
	return s.name
}

// Close implements [Reporter].
func (s *Stdout) Close() error {
	return nil
}

// Report writes only the document's tables as plain text.
func (s *Stdout) Report(_ context.Context, doc *report.Document) error {
	if doc == nil {
		return trace.BadParameter("document is required")
	}

	var b strings.Builder
	var tables int
	for _, section := range doc.Sections {
		if section.Table == nil {
			continue
		}
		if tables > 0 {
			b.WriteByte('\n')
		}
		s.writeTable(&b, section.Table)
		tables++
	}

	if _, err := io.WriteString(s.out, b.String()); err != nil {
		return trace.Wrap(err, "writing report %s", doc.ID)
	}
	return nil
}

// writeTable prints a table with per-column alignment.
func (s *Stdout) writeTable(b *strings.Builder, table *report.Table) {
	lines, shown, truncated := tsvTable(table, s.maxRows)

	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	if truncated {
		fmt.Fprintf(b, "... showing %d of %d\n", shown, table.TotalRows)
	}
}
