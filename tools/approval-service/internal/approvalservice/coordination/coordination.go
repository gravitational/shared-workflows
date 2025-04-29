package coordination

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var ErrAlreadyLeased = errors.New("lease already acquired")
var ErrInternalRateLimitExceeded = errors.New("internal rate limit exceeded")

// Coordinator handles coordination of work between different components and services.
type Coordinator struct {
	leaser leaser

	accessReuequestLeaseDur time.Duration

	// Rate limiter for GitHub workflows by WorkflowID
	workflowLeaseDur time.Duration
	rl               map[string]*rate.Limiter
	mu               sync.Mutex

	log *slog.Logger
}

type leaser interface {
	AcquireLease(ctx context.Context, id string, duration time.Duration) error
}

type Opt func(*Coordinator) error

func WithLogger(logger *slog.Logger) Opt {
	return func(c *Coordinator) error {
		if logger == nil {
			return fmt.Errorf("logger cannot be nil")
		}
		c.log = logger
		return nil
	}
}

func GitHubWorkflowLeaseDuration(d time.Duration) Opt {
	return func(c *Coordinator) error {
		if d <= 0 {
			return fmt.Errorf("invalid workflow lease duration: %v", d)
		}
		c.workflowLeaseDur = d
		return nil
	}
}

func AccessRequestLeaseDuration(d time.Duration) Opt {
	return func(c *Coordinator) error {
		if d <= 0 {
			return fmt.Errorf("invalid access request lease duration: %v", d)
		}
		c.accessReuequestLeaseDur = d
		return nil
	}
}

var defaultOpts = []Opt{
	WithLogger(slog.Default()),
	AccessRequestLeaseDuration(10 * time.Second),
	GitHubWorkflowLeaseDuration(1 * time.Minute),
}

func NewCoordinator(opts ...Opt) (*Coordinator, error) {
	var c Coordinator

	for _, opt := range append(defaultOpts, opts...) {
		if err := opt(&c); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	c.log.Info("Initializing coordinator")

	config, err := rest.InClusterConfig()
	switch {
	case errors.Is(err, rest.ErrNotInCluster):
		// TODO: Use a stub implementation for testing
		return nil, fmt.Errorf("not in cluster: %w", err)
	case err != nil:
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	ns, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return nil, fmt.Errorf("POD_NAMESPACE environment variable not set, use downwardAPI to set it")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	c.leaser = newKubeLeaser(clientset, ns, "holder-identity", c.log)

	c.rl = make(map[string]*rate.Limiter)

	return &c, nil
}

// LeaseGitHubWorkflow leases a GitHub workflow run for the given org, repo, and run ID.
// This will block until the lease is acquired or it is determined that the lease cannot be acquired (e.g. already leased).
//
// Rate limiting is applied to the workflow ID to prevent excessive lease acquisition attempts per service.
// The rate limit is set to 1 request per workflow lease duration.
// If the rate limit is exceeded, an error is returned.
func (c *Coordinator) LeaseGitHubWorkflow(ctx context.Context, org, repo string, runID int64) error {
	rl := c.getWorkflowRL(fmt.Sprintf("%s-%s-%d", org, repo, runID))
	if !rl.Allow() {
		return ErrInternalRateLimitExceeded
	}

	return c.leaser.AcquireLease(ctx, fmt.Sprintf("%s-%s-%d", org, repo, runID), c.workflowLeaseDur)
}

func (c *Coordinator) LeaseAccessRequest(ctx context.Context, id string) error {
	return c.leaser.AcquireLease(ctx, "request-"+id, c.workflowLeaseDur)
}

func (c *Coordinator) getWorkflowRL(id string) *rate.Limiter {
	c.mu.Lock()
	defer c.mu.Unlock()

	if rl, ok := c.rl[id]; ok {
		return rl
	}

	rl := rate.NewLimiter(rate.Every(c.workflowLeaseDur), 1)
	c.rl[id] = rl
	return rl
}
