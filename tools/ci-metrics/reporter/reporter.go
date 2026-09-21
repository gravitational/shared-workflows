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
	"io"
	"strings"

	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

// Reporter describes the interface for writing [report.Document]
type Reporter interface {
	Name() string
	Report(ctx context.Context, doc *report.Document) error
	io.Closer
}

// Preflighter is an optional interface for [Reporter] to implement
// a check before running queries.
type Preflighter interface {
	// Preflight verifies the configured reporter.
	Preflight(ctx context.Context) error
}

// Types returns every reporter type the config supports
func Types() []string {
	return []string{TypeStdout, TypeSlack}
}

// New builds the reporter described by [report.ReporterConfig].
func New(name string, cfg report.ReporterConfig, out io.Writer) (Reporter, error) {
	switch cfg.Type {
	case TypeStdout:
		return NewStdout(name, out, cfg.MaxRows), nil
	case TypeSlack:
		return NewSlack(name, cfg)
	default:
		return nil, trace.BadParameter(
			"reporter %s has unknown type %q, want one of %s",
			name, cfg.Type, strings.Join(Types(), ", "))
	}
}
