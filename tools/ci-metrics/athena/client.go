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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	athenasdk "github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/gravitational/trace"
)

// Client wraps [athenasdk.Client] with the tool [Config] and implements
// [Executor].
type Client struct {
	clt *athenasdk.Client
	cfg Config
}

var _ Executor = (*Client)(nil)

// NewFromConfig builds a [Client] from given [Config], uses ambient AWS credentials.
func NewFromConfig(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	// For some reason [awsconfig.LoadDefaultConfig] does not take `awsconfig.LoadOptionsFunc`
	var opts []func(*awsconfig.LoadOptions) error
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, trace.Wrap(err, "loading aws config")
	}

	return &Client{
		clt: athenasdk.NewFromConfig(awsCfg),
		cfg: cfg,
	}, nil
}

func (c *Client) workgroupOutputLocation(ctx context.Context) (string, error) {
	out, err := c.clt.GetWorkGroup(ctx, &athenasdk.GetWorkGroupInput{
		WorkGroup: aws.String(c.cfg.Workgroup),
	})
	if err != nil {
		return "", trace.Wrap(err, "fetching workgroup %s", c.cfg.Workgroup)
	}

	if out == nil ||
		out.WorkGroup == nil ||
		out.WorkGroup.Configuration == nil ||
		out.WorkGroup.Configuration.ResultConfiguration == nil {
		return "", nil
	}

	return aws.ToString(out.WorkGroup.Configuration.ResultConfiguration.OutputLocation), nil
}

// Execute runs a statement, blocks until it finishes, and reads back any rows
// it produced.
func (c *Client) Execute(ctx context.Context, statement string) (*Result, error) {
	result, err := c.start(ctx, statement)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if err := c.wait(ctx, result); err != nil {
		return nil, trace.Wrap(err)
	}

	if err := c.fetch(ctx, result); err != nil {
		return nil, trace.Wrap(err)
	}

	return result, nil
}

// start submits a statement and returns a new [Result]
func (c *Client) start(ctx context.Context, statement string) (*Result, error) {
	input := &athenasdk.StartQueryExecutionInput{
		QueryString: aws.String(statement),
		QueryExecutionContext: &types.QueryExecutionContext{
			Database: aws.String(c.cfg.Database),
		},
		WorkGroup: aws.String(c.cfg.Workgroup),
	}

	// Sending an empty ResultConfiguration is not the same as sending none, so
	// only set it when there is a location to send.
	if c.cfg.OutputLocation != "" {
		input.ResultConfiguration = &types.ResultConfiguration{
			OutputLocation: aws.String(c.cfg.OutputLocation),
		}
	} else {
		// Athena falls back to the workgroup's own result configuration, so
		// there is nothing to send here but we try our best to validate the config.
		// TODO(okraport): move this to only perform this preflight once.
		outputLocation, err := c.workgroupOutputLocation(ctx)
		if err != nil {
			return nil, trace.Wrap(err, "resolving output location for workgroup %s", c.cfg.Workgroup)
		}
		if outputLocation == "" {
			return nil, trace.BadParameter(
				"--results not specified and workgroup %s lacks default configuration", c.cfg.Workgroup)
		}
	}

	started, err := c.clt.StartQueryExecution(ctx, input)
	if err != nil {
		return nil, trace.Wrap(err, "starting query")
	}

	return &Result{QueryExecutionID: aws.ToString(started.QueryExecutionId)}, nil
}

// wait polls a running query until it reaches a terminal state.
func (c *Client) wait(ctx context.Context, result *Result) error {
	const defaultPollInterval = 2 * time.Second
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		out, err := c.clt.GetQueryExecution(ctx, &athenasdk.GetQueryExecutionInput{
			QueryExecutionId: aws.String(result.QueryExecutionID),
		})
		if err != nil {
			return trace.Wrap(err, "polling query %s", result.QueryExecutionID)
		}

		exec := out.QueryExecution
		if exec == nil || exec.Status == nil {
			return trace.BadParameter("query %s returned no status", result.QueryExecutionID)
		}

		switch exec.Status.State {
		case types.QueryExecutionStateSucceeded:
			result.DataScannedBytes = aws.ToInt64(exec.Statistics.DataScannedInBytes)
			result.EngineTime = time.Duration(aws.ToInt64(exec.Statistics.EngineExecutionTimeInMillis)) * time.Millisecond
			return nil

		case types.QueryExecutionStateFailed, types.QueryExecutionStateCancelled:
			return trace.Errorf("query %s %s: %s",
				result.QueryExecutionID,
				exec.Status.State,
				aws.ToString(exec.Status.StateChangeReason),
			)
		}

		select {
		case <-ctx.Done():
			return trace.Wrap(ctx.Err(), "waiting on query %s", result.QueryExecutionID)
		case <-ticker.C:
		}
	}
}

// fetch pages through the results of a completed statement.
func (c *Client) fetch(ctx context.Context, result *Result) error {
	pager := athenasdk.NewGetQueryResultsPaginator(c.clt, &athenasdk.GetQueryResultsInput{
		QueryExecutionId: aws.String(result.QueryExecutionID),
	})

	// Athena repeats the column names as the first row of the first page because it paginates
	// the result file, which is a CSV file in s3. This is not explicitly called out in the docs.
	// Furthermore the column information is already carried in a seperate struct so we simply discard it.
	var headerDropped bool

	for pager.HasMorePages() {
		if ctx.Err() != nil {
			return trace.Wrap(ctx.Err())
		}

		out, err := pager.NextPage(ctx)
		if err != nil {
			return trace.Wrap(err, "fetching results for query %s", result.QueryExecutionID)
		}

		if out.ResultSet == nil {
			continue
		}

		// Populate column names once if possible.
		if len(result.Columns) == 0 && out.ResultSet.ResultSetMetadata != nil {
			for _, ci := range out.ResultSet.ResultSetMetadata.ColumnInfo {
				result.Columns = append(result.Columns, aws.ToString(ci.Name))
			}
		}

		rows := out.ResultSet.Rows
		if !headerDropped && len(rows) > 0 {
			rows = rows[1:]
			headerDropped = true
		}

		for _, r := range rows {
			row := make(Row, 0, len(r.Data))
			for _, d := range r.Data {
				row = append(row, d.VarCharValue)
			}
			result.Rows = append(result.Rows, row)
		}

		if c.cfg.MaxRows > 0 && len(result.Rows) > c.cfg.MaxRows {
			return trace.LimitExceeded(
				"query %s returned more than %d rows", result.QueryExecutionID, c.cfg.MaxRows)
		}
	}

	return nil
}
