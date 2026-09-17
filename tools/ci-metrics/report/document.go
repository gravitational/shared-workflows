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

package report

import "time"

// Document is the rendering of one report.
//
// Cells and metrics carry text that is already formatted for display.
type Document struct {
	// ID is the report name, stable across runs.
	ID string
	// Title names the report.
	Title string
	// Generally reports tables and window this report covers.
	Subtitle string
	// Headline is a short summary for reporters that can print summaries (eg: slack)
	Headline string
	// Sections are the main body of the report.
	Sections []Section
	// Meta contains metadata about this doc.
	Meta DocMeta
}

// DocMeta contains metadata about this doc.
type DocMeta struct {
	// From and To bound the reporting window, inclusive.
	From, To time.Time
	// GeneratedAt is when the document was built.
	GeneratedAt time.Time
	// DataScannedBytes totals the bytes Athena scanned across every query the
	// report ran.
	DataScannedBytes int64
	// QueryExecutionIDs lists the Athena executions behind the document, for
	// tracing a surprising number back to its query.
	QueryExecutionIDs []string
}

// Section is one titled block of a [Document]. The fields are independently
// optional; a reporter renders whichever are set.
type Section struct {
	Heading string
	Text    string
	Metrics []Metric
	Table   *Table
	Notes   []string
}

// Table is a rectangular block of cells.
type Table struct {
	Columns []Column
	Rows    []Row
	// TotalRows is how many rows the underlying query matched
	TotalRows int
}

// Row is one row of a [Table], positionally aligned with [Table.Columns].
type Row []Cell

// Align is the horizontal alignment of a column.
type Align int

const (
	// AlignLeft suits text.
	AlignLeft Align = iota
	// AlignRight suits numbers, so digits line up by place value.
	AlignRight
)

// Column describes one column of a [Table].
type Column struct {
	Name  string
	Align Align
}

// Severity is the significance of a [Cell], which reporters may show as
// colour or a glyph.
type Severity int

const (
	SeverityNone Severity = iota
	SeverityOK
	SeverityWarn
	SeverityBad
)

// Cell is one value of a [Table], formatted for display.
type Cell struct {
	Text     string
	Link     string
	Severity Severity
}

// Metric is a single named figure, optionally with its movement since the
// previous comparable window.
type Metric struct {
	Name  string
	Value string
	Delta *Delta
}

// Delta is the movement of a [Metric].
type Delta struct {
	// Text is the change, already formatted, such as "+12%".
	Text string
	// Up reports whether the value rose.
	Up bool
	// UpIsGood distinguishes a rising pass rate from a rising failure rate, so
	// a reporter can colour the movement without knowing the metric.
	UpIsGood bool
}
