package athena

import (
	"context"
	"fmt"
	"os"

	"github.com/gravitational/trace"
)

var _ Executor = (*NoopClient)(nil)

// NoopClient prints the statements requested without running anything.
type NoopClient struct {
	cfg Config
}

// NewNoop builds a client that prints statements to [os.Stderr] instead of
// running them.
func NewNoop(_ context.Context, cfg Config) (*NoopClient, error) {
	if err := cfg.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &NoopClient{cfg: cfg}, nil
}

// Execute prints the statement and returns an empty result
func (c *NoopClient) Execute(_ context.Context, statement string) (*Result, error) {
	fmt.Fprintf(os.Stderr, "\n-- DRYRUN:\n%s\n", statement)
	return &Result{}, nil
}
