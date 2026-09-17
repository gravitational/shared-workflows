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

package athena

import (
	"context"
	"strings"
	"time"

	"github.com/gravitational/trace"
)

// defaultMaxRows bounds how many rows [Client.Query] will accumulate. Reports
// are top-N aggregates, so a result set larger than this means the query is
// wrong rather than the limit being too low.
const defaultMaxRows = 10_000

type Config struct {
	Database  string
	Workgroup string
	Region    string

	// OutputLocation is the S3 prefix for query results. Athena rejects
	// StartQueryExecution unless this is either specified or configured for the workgroup.
	OutputLocation string

	// MaxRows bounds the rows [Client.Query] will read. Defaults to
	// [defaultMaxRows]; a negative value means no limit.
	MaxRows int
}

func (c *Config) checkAndSetDefaults() error {
	if c.Database == "" {
		return trace.BadParameter("database is required")
	}
	if c.Workgroup == "" {
		c.Workgroup = "primary"
	}
	if c.OutputLocation != "" && !strings.HasPrefix(c.OutputLocation, "s3://") {
		return trace.BadParameter("output location %q must be an s3:// URI", c.OutputLocation)
	}
	if c.MaxRows == 0 {
		c.MaxRows = defaultMaxRows
	}
	return nil
}

// Result describes the outcome of running a statement against Athena: the
// execution metadata, and the rows if the statement produced any.
type Result struct {
	QueryExecutionID string
	DataScannedBytes int64
	EngineTime       time.Duration

	// Columns names the columns in positional order
	Columns []string
	// Rows holds the data rows
	Rows []Row
}

// Executor runs a statement and blocks until it finishes.
type Executor interface {
	Execute(ctx context.Context, statement string) (*Result, error)
}
