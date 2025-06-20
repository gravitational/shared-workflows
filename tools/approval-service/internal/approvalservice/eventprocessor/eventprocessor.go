package eventprocessor

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/coordination"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/eventprocessor/store"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/githubevents"
	"github.com/gravitational/teleport/api/types"
	"golang.org/x/sync/errgroup"
)

// Processor manages changes to the state of Access Requests and their relation to deployment events (e.g. GitHub deployment review events).
// It acts as a broker for the various event sources, ensuring that events are processed by the appropriate consumers.
type Processor struct {
	store store.ProcessorService

	teleportUser    string
	requestTTLHours time.Duration

	// githubConsumers is a map of GitHub source processors.
	// The key is a string of the form "org/repo" and the value is the GitHub source processor.
	// This is used to look up the GitHub source processor for a given GitHub event or Access Request.
	// not safe for concurrent read/write
	// this is written to during init and only read concurrently during operation.
	githubConsumers    map[string]*GitHubConsumer
	deployReviewEventC chan githubevents.DeploymentReviewEvent

	coordinator coordination.Coordinator

	log *slog.Logger
}

// Consumers contains the a subset of processors that are used to handle events from different sources.
// They are responsible for doing the actual work of processing the events and updating the state of Access Requests.
type Consumers struct {
	GitHub []*GitHubConsumer
}

// Opt is a functional option for configuring the Processor.
type Opt func(p *Processor) error

// WithLogger sets the logger for the Processor.
func WithLogger(logger *slog.Logger) Opt {
	return func(p *Processor) error {
		if logger == nil {
			return errors.New("logger cannot be nil")
		}
		p.log = logger
		return nil
	}
}

// New creates a new Processor instance.
func New(ctx context.Context, teleConfig config.Teleport, sp *Consumers, coordinator coordination.Coordinator, opts ...Opt) (*Processor, error) {
	p := &Processor{
		teleportUser:    teleConfig.User,
		requestTTLHours: cmp.Or(time.Duration(teleConfig.RequestTTLHours)*time.Hour, 7*24*time.Hour),
		githubConsumers: make(map[string]*GitHubConsumer),
		coordinator:     coordinator,
		log:             slog.Default(),
	}

	for _, opt := range opts {
		if err := opt(p); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	for _, gh := range sp.GitHub {
		p.log.Info("Registering GitHub consumers", "id", githubID(gh.Org, gh.Repo))
		p.githubConsumers[githubID(gh.Org, gh.Repo)] = gh
	}

	return p, nil
}

// Run starts the Processor and starts receiving events from the event sources.
func (p *Processor) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return p.githubEventListener(ctx)
	})

	return eg.Wait()
}

// HandleReview will handle updates to the state of a Teleport Access Request.
func (p *Processor) HandleReview(ctx context.Context, req types.AccessRequest) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := p.coordinator.LeaseAccessRequest(ctx, req.GetName()); err != nil {
		return err
	}

	// Currently only GitHub is supported.
	// If another source is added, will need to be updated to serialize the processor type to delegate to.
	return p.handleGitHubReview(ctx, req)
}
