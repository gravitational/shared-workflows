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
	"strings"

	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

var (
	_ Reporter    = (Multi)(nil)
	_ Preflighter = (Multi)(nil)
)

// Multi fans a document out to several reporters.
type Multi []Reporter

// Name returns the names of the wrapped reporters.
func (m Multi) Name() string {
	names := make([]string, 0, len(m))
	for _, r := range m {
		names = append(names, r.Name())
	}
	return strings.Join(names, ",")
}

// Report writes the document to every reporter
func (m Multi) Report(ctx context.Context, doc *report.Document) error {
	var errs []error
	for _, r := range m {
		if err := r.Report(ctx, doc); err != nil {
			errs = append(errs, trace.Wrap(err, "reporter %s", r.Name()))
		}
	}
	return trace.NewAggregate(errs...)
}

// Preflight checks every wrapped reporter that supports it.
func (m Multi) Preflight(ctx context.Context) error {
	var errs []error
	for _, r := range m {
		p, ok := r.(Preflighter)
		if !ok {
			continue
		}
		if err := p.Preflight(ctx); err != nil {
			errs = append(errs, trace.Wrap(err, "reporter %s", r.Name()))
		}
	}
	return trace.NewAggregate(errs...)
}

// Close closes every reporter, collecting failures.
func (m Multi) Close() error {
	var errs []error
	for _, r := range m {
		if err := r.Close(); err != nil {
			errs = append(errs, trace.Wrap(err, "reporter %s", r.Name()))
		}
	}
	return trace.NewAggregate(errs...)
}
