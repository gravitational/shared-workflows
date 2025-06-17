package eventprocessor

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/eventprocessor/store"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/githubevents"
	"github.com/gravitational/teleport/api/types"
	"golang.org/x/sync/errgroup"
)

// Processor manages changes to the state of Access Requests and their relation to deployment events (e.g. GitHub deployment review events).
// It delegates the actual processing of the events to the appropriate processor configured in [SourceProcessors].
// It's primary purpose is to ensure that events are handled by the appropriate processor.
type Processor struct {
	sp *SourceProcessors

	store store.ProcessorService

	teleportUser    string
	requestTTLHours time.Duration

	// githubProcessors is a map of GitHub source processors.
	// The key is a string of the form "org/repo" and the value is the GitHub source processor.
	// This is used to look up the GitHub source processor for a given GitHub event or Access Request.
	// not safe for concurrent read/write
	// this is written to during init and only read concurrently during operation.
	githubProcessors   map[string]*GitHubSourceProcessor
	deployReviewEventC chan githubevents.DeploymentReviewEvent

	coordinator Coordinator

	log *slog.Logger
}

// Coordinator is an interface that defines methods for coordinating tasks among multiple instances of the approval service.
// Mainly used for ensuring that only one instance of the approval service is handling a specific task at a time.
type Coordinator interface {
	LeaseAccessRequest(ctx context.Context, id string) error
	LeaseGitHubWorkflow(ctx context.Context, org, repo string, workflowID int64) error
}

// SourceProcessors contains the a subset of processors that are used to handle events from different sources.
// They are responsible for doing the actual work of processing the events and updating the state of Access Requests.
type SourceProcessors struct {
	GitHub []*GitHubSourceProcessor
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
func New(ctx context.Context, teleConfig config.Teleport, sp *SourceProcessors, coordinator Coordinator, opts ...Opt) (*Processor, error) {
	p := &Processor{
		teleportUser:     teleConfig.User,
		sp:               sp,
		requestTTLHours:  cmp.Or(time.Duration(teleConfig.RequestTTLHours)*time.Hour, 7*24*time.Hour),
		githubProcessors: make(map[string]*GitHubSourceProcessor),
		coordinator:      coordinator,
		log:              slog.Default(),
	}

	for _, opt := range opts {
		if err := opt(p); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	for _, gh := range p.sp.GitHub {
		p.log.Info("Registering GitHub source processor", "id", githubID(gh.Org, gh.Repo))
		p.githubProcessors[githubID(gh.Org, gh.Repo)] = gh
	}

	return p, nil
}

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
