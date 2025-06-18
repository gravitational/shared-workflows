package coordination

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type Coordinator interface {
	// LeaseGitHubWorkflow leases a GitHub workflow run for the given org, repo, and run ID.
	// This will block until the lease is acquired or it is determined that the lease cannot be acquired (e.g. already leased).
	//
	// Rate limiting is applied to the workflow ID to prevent excessive lease acquisition attempts per service.
	// The rate limit is set to 1 request per workflow lease duration.
	// If the rate limit is exceeded, an error is returned.
	LeaseGitHubWorkflow(ctx context.Context, org, repo string, runID int64) error

	// LeaseAccessRequest leases an access request for the given ID.
	// This prevents multiple services from processing the same access request concurrently.
	//
	// This will block until the lease is acquired or it is determined that the lease cannot be acquired (e.g. already leased).
	LeaseAccessRequest(ctx context.Context, id string) error
}

var ErrInternalRateLimitExceeded = errors.New("internal rate limit exceeded")

type coordinator struct {
	holderIdentity string
	leaser         leaser

	accessRequestLeaseDur time.Duration

	// Rate limiter for GitHub workflows by WorkflowID
	workflowLeaseDur time.Duration
	rl               map[string]*rate.Limiter
	mu               sync.Mutex

	kubeInitFunc func() (kubernetes.Interface, string, error)

	log *slog.Logger
}

type leaser interface {
	AcquireLease(ctx context.Context, leaseName string, duration time.Duration) error
}

func NewCoordinator(opts ...Opt) (Coordinator, error) {
	c := &coordinator{
		workflowLeaseDur:      10 * time.Second, // Default lease duration for GitHub workflows
		accessRequestLeaseDur: 10 * time.Second, // Default lease duration for access requests
		log:                   slog.Default(),
		holderIdentity:        uuid.NewString(),
		kubeInitFunc:          inClusterKubeInit,
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	c.log.Info("Initializing coordinator")

	clientset, ns, err := c.kubeInitFunc()
	switch {
	case errors.Is(err, rest.ErrNotInCluster):
		// TODO: Use a stub implementation for testing
		return nil, fmt.Errorf("not in cluster: %w", err)
	case err != nil:
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	c.leaser = &kubeleaser{
		namespace:      ns,
		client:         clientset,
		log:            c.log,
		holderIdentity: c.holderIdentity,
	}

	c.rl = make(map[string]*rate.Limiter)

	return c, nil
}

func (c *coordinator) LeaseGitHubWorkflow(ctx context.Context, org, repo string, runID int64) error {
	rl := c.getWorkflowRL(fmt.Sprintf("%s-%s-%d", org, repo, runID))
	if !rl.Allow() {
		return ErrInternalRateLimitExceeded
	}

	return c.leaser.AcquireLease(ctx, fmt.Sprintf("%s-%s-%d", org, repo, runID), c.workflowLeaseDur)
}

func (c *coordinator) LeaseAccessRequest(ctx context.Context, id string) error {
	return c.leaser.AcquireLease(ctx, "request-"+id, c.accessRequestLeaseDur)
}

func (c *coordinator) getWorkflowRL(id string) *rate.Limiter {
	c.mu.Lock()
	defer c.mu.Unlock()

	if rl, ok := c.rl[id]; ok {
		return rl
	}

	rl := rate.NewLimiter(rate.Every(c.workflowLeaseDur), 1)
	c.rl[id] = rl
	return rl
}

// inClusterKubeInit is a function that initializes a Kubernetes client in-cluster.
// This is the default function used by the coordinator to get the Kubernetes client and namespace.
var inClusterKubeInit = func() (kubernetes.Interface, string, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	ns, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return nil, "", fmt.Errorf("POD_NAMESPACE environment variable not set, use downwardAPI to set it")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return clientset, ns, nil
}

type kubeleaser struct {
	namespace      string
	client         kubernetes.Interface
	holderIdentity string

	log *slog.Logger
}

func (k *kubeleaser) AcquireLease(ctx context.Context, leaseName string, duration time.Duration) error {
	acq := make(chan bool)

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{Name: leaseName, Namespace: k.namespace},
		Client:    k.client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: k.holderIdentity,
		},
	}

	lec, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   duration,
		RenewDeadline:   duration / 2,
		RetryPeriod:     time.Second * 2,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				k.log.Info("Acquired lease", "lease", leaseName)
				select {
				case acq <- true:
				default:
				}
			},
			OnStoppedLeading: func() {
				k.log.Info("Lost lease", "lease", leaseName)
			},
		},
	})
	if err != nil {
		return err
	}

	go func() {
		lec.Run(ctx)
	}()

	select {
	case <-acq:
		return nil
	case <-ctx.Done():
		k.log.Error("Context canceled while waiting for lease acquisition", "lease", leaseName)
		return ctx.Err()
	}
}

type Opt func(*coordinator) error

func WithLogger(logger *slog.Logger) Opt {
	return func(c *coordinator) error {
		if logger == nil {
			return fmt.Errorf("logger cannot be nil")
		}
		c.log = logger
		return nil
	}
}

func GitHubWorkflowLeaseDuration(d time.Duration) Opt {
	return func(c *coordinator) error {
		if d <= 0 {
			return fmt.Errorf("invalid workflow lease duration: %v", d)
		}
		c.workflowLeaseDur = d
		return nil
	}
}

func AccessRequestLeaseDuration(d time.Duration) Opt {
	return func(c *coordinator) error {
		if d <= 0 {
			return fmt.Errorf("invalid access request lease duration: %v", d)
		}
		c.accessRequestLeaseDur = d
		return nil
	}
}

func withKubeInitFunc(f func() (kubernetes.Interface, string, error)) Opt {
	return func(c *coordinator) error {
		if f == nil {
			return fmt.Errorf("kube init func cannot be nil")
		}
		c.kubeInitFunc = f

		return nil
	}
}

func withHolderIdentity(id string) Opt {
	return func(c *coordinator) error {
		if id == "" {
			return fmt.Errorf("holder identity cannot be empty")
		}
		c.holderIdentity = id
		return nil
	}
}
