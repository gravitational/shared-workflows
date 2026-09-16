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
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	athenasdk "github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/gravitational/trace"
)

type Config struct {
	Database  string
	Workgroup string
	Region    string

	// OutputLocation is the S3 prefix for query results. Athena rejects
	// StartQueryExecution unless this is etiher specified or configured for the workgroup.
	OutputLocation string
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
	return nil
}

// Result describes the outcome of running a query against Athena
type Result struct {
	QueryExecutionID string
	DataScannedBytes int64
	EngineTime       time.Duration
}

type Executor interface {
	Execute(ctx context.Context, statement string) (*Result, error)
}

// Client wraps [athenasdk.Client] with the tool [Config], implements [Executor]
type Client struct {
	clt *athenasdk.Client
	cfg Config
}

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

// Execute runs a statement and blocks until done.
func (c *Client) Execute(ctx context.Context, statement string) (*Result, error) {
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
		// TODO(okraport): move this to only perform this preflight once.
		outputLocation, err := c.workgroupOutputLocation(ctx)
		if err != nil || outputLocation == "" {
			return nil, trace.NewAggregate(
				trace.BadParameter("--results not specified and workgroup lacks default configuration"),
				trace.Wrap(err))
		}
	}

	started, err := c.clt.StartQueryExecution(ctx, input)
	if err != nil {
		return nil, trace.Wrap(err, "starting query")
	}
	id := aws.ToString(started.QueryExecutionId)

	const defaultPollInterval = 2 * time.Second
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		out, err := c.clt.GetQueryExecution(ctx, &athenasdk.GetQueryExecutionInput{
			QueryExecutionId: aws.String(id),
		})
		if err != nil {
			return nil, trace.Wrap(err, "polling query %s", id)
		}

		exec := out.QueryExecution
		if exec == nil || exec.Status == nil {
			return nil, trace.BadParameter("query %s returned no status", id)
		}

		switch exec.Status.State {
		case types.QueryExecutionStateSucceeded:
			return newResult(id, exec.Statistics), nil

		case types.QueryExecutionStateFailed, types.QueryExecutionStateCancelled:
			return nil, trace.Errorf("query %s %s: %s",
				id,
				exec.Status.State,
				aws.ToString(exec.Status.StateChangeReason),
			)
		}

		select {
		case <-ctx.Done():
			return nil, trace.Wrap(ctx.Err(), "waiting on query %s", id)
		case <-ticker.C:
		}
	}
}

func newResult(id string, stats *types.QueryExecutionStatistics) *Result {
	r := &Result{QueryExecutionID: id}
	if stats != nil {
		r.DataScannedBytes = aws.ToInt64(stats.DataScannedInBytes)
		r.EngineTime = time.Duration(aws.ToInt64(stats.EngineExecutionTimeInMillis)) * time.Millisecond
	}
	return r
}

// NoopClient prints the statements requested without running anything.
type NoopClient struct {
	cfg Config
}

func NewNoop(ctx context.Context, cfg Config) (*NoopClient, error) {
	if err := cfg.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	return &NoopClient{
		cfg: cfg,
	}, nil
}

func (c *NoopClient) Execute(_ context.Context, statement string) (*Result, error) {
	fmt.Printf("\n-- DRYRUN:\n%s\n", statement)
	return &Result{}, nil
}
