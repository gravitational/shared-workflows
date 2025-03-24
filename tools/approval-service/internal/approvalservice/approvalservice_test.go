package approvalservice

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/githubevents"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestApprovalService(t *testing.T) {
	testApprovalService, processor, ghEvents := newTestApprovalService(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return testApprovalService.Run(ctx)
	})

	t.Run("Receive Deployment Event - Create Access Request", func(t *testing.T) {
		firstWorkflowID := int64(123456)
		secondWorkflowID := int64(654321)
		thirdWorkflowID := int64(987654)

		ghEvents.emitEvents([]githubevents.DeploymentReviewEvent{
			{
				Environment:  "build/stage",
				Repository:   "gravitational/teleport",
				Organization: "gravitational",
				Requester:    "test-user",
				WorkflowID:   firstWorkflowID,
			},
			{
				Environment:  "build/stage",
				Repository:   "gravitational/teleport",
				Organization: "gravitational",
				Requester:    "test-user",
				WorkflowID:   secondWorkflowID,
			},
			{
				Environment:  "build/stage",
				Repository:   "gravitational/teleport",
				Organization: "gravitational",
				Requester:    "test-user",
				WorkflowID:   thirdWorkflowID,
			},
		})
		assert.NoError(t, <-processor.waitForWorkflowID(ctx, firstWorkflowID))
		assert.NoError(t, <-processor.waitForWorkflowID(ctx, secondWorkflowID))
		assert.NoError(t, <-processor.waitForWorkflowID(ctx, thirdWorkflowID))
	})
	cancel()
	require.NoError(t, eg.Wait())
}

func newTestApprovalService(t *testing.T) (app *ApprovalService, proc *fakeProcessor, ghEvents *fakeGitHubEventSource) {
	proc = &fakeProcessor{
		workflowIDs: make(map[int64]struct{}),
	}
	ghEvents = &fakeGitHubEventSource{
		processor: proc,
		eventC:    make(chan githubevents.DeploymentReviewEvent),
	}
	app, err := newWithOpts()
	require.NoError(t, err)
	app.eventSources = []EventSource{
		ghEvents,
	}
	app.processor = proc

	return app, proc, ghEvents
}

type fakeGitHubEventSource struct {
	eventC    chan githubevents.DeploymentReviewEvent
	processor EventProcessor
}

func (f *fakeGitHubEventSource) emitEvents(events []githubevents.DeploymentReviewEvent) {
	for _, event := range events {
		f.eventC <- event
	}
}

func (f *fakeGitHubEventSource) Setup() error {
	return nil
}

func (f *fakeGitHubEventSource) Run(ctx context.Context) error {
	defer close(f.eventC)
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-f.eventC:
			go f.processor.ProcessDeploymentReviewEvent(e, true)
		}
	}
}

type fakeProcessor struct {
	mu          sync.Mutex
	workflowIDs map[int64]struct{}
}

var _ EventProcessor = &fakeProcessor{}

func (f *fakeProcessor) Setup() error {
	return nil
}

func (f *fakeProcessor) ProcessDeploymentReviewEvent(e githubevents.DeploymentReviewEvent, valid bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workflowIDs[e.WorkflowID] = struct{}{}
	return nil
}

func (f *fakeProcessor) HandleReview(ctx context.Context, req types.AccessRequest) error {
	return nil
}

func (f *fakeProcessor) waitForWorkflowID(ctx context.Context, workflowID int64) <-chan error {
	tick := time.NewTicker(100 * time.Millisecond)
	errC := make(chan error)
	go func() {
		defer tick.Stop()
		defer close(errC)
		for {
			select {
			case <-ctx.Done():
				errC <- fmt.Errorf("context cancelled before finding workflow ID %d", workflowID)
				return
			case <-tick.C:
				f.mu.Lock()
				_, ok := f.workflowIDs[workflowID]
				f.mu.Unlock()
				if ok {
					return
				}
			}
		}
	}()

	return errC
}
