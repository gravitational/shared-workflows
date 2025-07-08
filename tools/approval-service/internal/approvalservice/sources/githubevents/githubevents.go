/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package githubevents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/go-github/v71/github"
	"github.com/gravitational/shared-workflows/libs/github/webhook"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
)

// Source is a webhook that listens for GitHub events and processes them.
// This implements the [http.Handler] interface and can be used as a standard HTTP handler.
type Source struct {
	handler                   http.Handler
	processor                 GitHubEventProcessor
	payloadValidationDisabled bool

	log *slog.Logger
}

// GitHubEventProcessor is an interface for processing deployment review events.
type GitHubEventProcessor interface {
	// ProcessDeploymentReviewEvent processes a deployment review event.
	ProcessDeploymentReviewEvent(ctx context.Context, event DeploymentReviewEvent) error

	// ProcessWorkflowDispatchEvent processes a workflow dispatch event.
	ProcessWorkflowDispatchEvent(ctx context.Context, event WorkflowDispatchEvent) error
}

// DeploymentReviewEvent is an event that is sent when a deployment review is requested.
// This contains information needed to process a request
type DeploymentReviewEvent struct {
	// WorkflowID is the ID of the workflow that is being reviewed.
	WorkflowID int64
	// Environment is the environment that is being reviewed.
	Environment string

	// Requester is the user that requested the deployment review.
	Requester string
	// Organization is the organization that owns the repository.
	Organization string
	// Repository is the repository that owns the deployment review.
	Repository string
}

// WorkflowDispatchEvent is an event that is sent when a workflow is dispatched.
// This primarily contains the inputs for the workflow dispatch.
type WorkflowDispatchEvent struct {
	// Inputs are the inputs for the workflow dispatch.
	// Despite workflow inputs supporting multiple types, this is always given an unstructured map of string to string.
	Inputs map[string]string

	// Requester is the user that requested the workflow dispatch.
	Requester string
	// Organization is the organization that owns the repository.
	Organization string
	// Repository is the repository that owns the workflow dispatch.
	Repository string
}

// Opt is a functional option for the GitHub event source.
type SourceOpt func(*Source) error

// WithLogger sets the logger for the GitHub event source.
func WithLogger(logger *slog.Logger) SourceOpt {
	return func(s *Source) error {
		if logger == nil {
			return errors.New("logger cannot be nil")
		}
		s.log = logger
		return nil
	}
}

// NewSource creates a new GitHub event source.
func NewSource(cfg config.GitHubSource, processor GitHubEventProcessor, opt ...SourceOpt) (*Source, error) {
	if processor == nil {
		return nil, errors.New("processor cannot be nil")
	}

	ghes := &Source{
		log:       slog.Default(),
		processor: processor,
	}

	for _, o := range opt {
		if err := o(ghes); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	opts := []webhook.Opt{
		webhook.WithLogger(ghes.log),
	}
	if ghes.payloadValidationDisabled {
		opts = append(opts, webhook.WithoutPayloadValidation())
	} else {
		opts = append(opts, webhook.WithSecretToken(cfg.Secret))
	}

	wh, err := webhook.NewHandler(ghes, opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating webhook handler: %w", err)
	}
	ghes.handler = wh

	return ghes, nil
}

// Handler returns the HTTP handler for the GitHub event source.
func (ghes *Source) Handler() http.Handler {
	return ghes.handler
}

// assert that Source implements the webhook.EventHandler interface
var _ webhook.EventHandler = (*Source)(nil)

// HandleEvent implements the webhook.EventHandler interface
func (ghes *Source) HandleEvent(ctx context.Context, event any) error {
	switch event := event.(type) {
	case *github.DeploymentProtectionRuleEvent:
		return ghes.processDeploymentReviewEvent(ctx, event)
	case *github.WorkflowDispatchEvent:
		return errors.New("workflow_dispatch not implemented")
	default:
		ghes.log.Debug("unknown event type", "type", fmt.Sprintf("%T", event))
	}
	return nil
}

// Process a deployment review event.
// This is where most of the business logic will go.
func (ghes *Source) processDeploymentReviewEvent(ctx context.Context, payload *github.DeploymentProtectionRuleEvent) error {
	workflowID, err := payload.GetRunID()
	if err != nil {
		return fmt.Errorf("error extracting workflow ID from callback URL: %w", err)
	}

	event := DeploymentReviewEvent{
		Requester:    payload.GetDeployment().GetCreator().GetLogin(),
		Environment:  payload.GetEnvironment(),
		Organization: payload.GetRepo().GetOwner().GetLogin(),
		Repository:   payload.GetRepo().GetName(),
		WorkflowID:   workflowID,
	}
	ghes.log.Info("received event", "event", event)

	// Process the event
	return ghes.processor.ProcessDeploymentReviewEvent(ctx, event)
}

// LogValue represents the DeploymentReviewEvent in a structured log format.
func (e DeploymentReviewEvent) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("requester", e.Requester),
		slog.String("environment", e.Environment),
		slog.String("organization", e.Organization),
		slog.String("repository", e.Repository),
		slog.Int64("workflow_id", e.WorkflowID),
	)
}
