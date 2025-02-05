package main

// Process:
// 1. Take in events from CI/CD systems
// 2. Extract common information
// 3. Process information according to business rules/logic
// 4. Callback to the event source, have it handle

// One of the design goals of this is to support multiple "sources" of deployment events,
// such as github or another CI/CD service.

// Skeleton TODO:
// * add ctx where needed
// * err handling
// * pass some form of "config" struct to setup funcs, which will be populated by CLI or config file
// * maybe add some "hook" for registering CLI options?
// * Move approval processor, event, and event source to different packages

func main() {
	// 0. Process CLI args, setup logger, etc.
	// TODO

	// 1. Setup approval processor
	var processor ApprovalProcessor = &TeleportApprovalProcessor{}
	_ = processor.Setup() // Error handling TODO

	// 2. Setup event sources
	eventSources := []EventSource{
		NewGitHubEventSource(processor),
	}

	for _, eventSource := range eventSources {
		_ = eventSource.Setup()
	}

	done := make(chan struct{}) // TODO replace with error?
	for _, eventSource := range eventSources {
		_ = eventSource.Run(done)
	}

	// Block until an event source has a fatal error
	<-done
	close(done)
}

// This contains information needed to process a request
// If multiple underlying events/payloads/etc. roll under
// the same "root" event that approval is for, they should
// all set the same ID.
type Event struct {
	// Unique for an approval request, but may be common for
	// multiple underlying events/payloads/etc.
	ID        string
	Requester string
	// Other fields TODO. Potential fields:
	// * Source
	// * Commit/tag/source control identifier
	// * "Parameters" map. In the case of GHA, this would be
	//   any input provided to a workflow dispatch event
	// See RFD for more details
}

type ApprovalProcessor interface {
	// This should do things like setup API clients, as well as anything
	// needed to approve/deny events.
	Setup() error

	// This should be a blocking function that takes in an event, and
	// approves or denies it.
	ProcessRequest(*Event) (approved bool, err error)
}

type TeleportApprovalProcessor struct {
	// TODO
}

func (tap *TeleportApprovalProcessor) Setup() error {
	// Setup Teleport API client
	return nil
}

func (tap *TeleportApprovalProcessor) ProcessRequest(e *Event) (approved bool, err error) {
	// 1. Create a new role:
	// 	* Set TTL to value in RFD
	// 	* Encode event information in role for recordkeeping

	// 2. Request access to the role. Include the same info as the role,
	//    for reviewer visibility.

	// 3. Wait for the request to be approved or denied.
	// This may block for a long time (minutes, hours, days).
	// Timeout if it takes too long.

	return false, nil
}

type EventSource interface {
	// This should do thinks like setup API clients and webhooks.
	Setup() error

	// Handle actual requests. This should not block.
	Run(chan struct{}) error
}

type GitHubEventSource struct {
	processor ApprovalProcessor
	// TODO
}

func NewGitHubEventSource(processor ApprovalProcessor) *GitHubEventSource {
	return &GitHubEventSource{processor: processor}
}

// Setup GH client, webhook secret, etc.
// https://github.com/go-playground/webhooks may help here
func (ghes *GitHubEventSource) Setup() error {
	// TODO
	return nil
}

// Take incoming events and respond to them
func (ghes *GitHubEventSource) Run(done chan struct{}) error {
	// If anything errors, deny the request. For safety, maybe `defer`
	// the "response" function?
	go func() {
		// Notify the service that the listener is completely done.
		// Normally this should only be hit if there is a fatal error
		defer func() { done <- struct{}{} }()

		// Incoming webhook payloads
		// This should be closed by the webhook listener func
		payloads := make(chan interface{})

		// Listen for webhook calls
		go ghes.listenForPayloads(payloads, done)

		for payload := range payloads {
			go ghes.processWebhookPayload(payload, done)
		}
	}()

	return nil
}

// Listen for incoming webhook events. Register HTTP routes, start server, etc. Long running, blocking.
func (ghes *GitHubEventSource) listenForPayloads(payloads chan interface{}, done chan struct{}) {
	// Once a call is received, it should return a 200 response immediately.

	// TODO
}

// Given an event, approve or deny it. This is a long running, blocking function.
func (ghes *GitHubEventSource) processWebhookPayload(payload interface{}, done chan struct{}) {
	// Do GitHub-specific checks. Don't approve based off ot this - just deny
	// if one fails.
	automatedDenial, err := ghes.performAutomatedChecks(payload)
	if automatedDenial || err != nil {
		ghes.respondToDeployRequest(false, payload)
	}

	// Convert it to a generic event that is common to all sources
	event := ghes.convertWebhookPayloadToEvent(payload)

	// Process the event
	processorApproved, err := ghes.processor.ProcessRequest(event)

	// Respond to the event
	if !processorApproved || err != nil {
		ghes.respondToDeployRequest(false, payload)
	}

	_ = ghes.respondToDeployRequest(true, payload)
}

// Turns GH-specific information into "common" information for the approver
func (ghes *GitHubEventSource) convertWebhookPayloadToEvent(payload interface{}) *Event {
	// This needs to perform logic to get the top-level workflow identifier. For example if
	// workflow job A calls workflow B, events for both A and B should use A's ID as the
	// event identifier

	return &Event{}
}

// Performs approval checks that are GH-specific. This should only be used to deny requests,
// never approve them.
func (ghes *GitHubEventSource) performAutomatedChecks(payload interface{}) (pass bool, err error) {
	// Verify request is from Gravitational org repo
	// Verify request is from Gravitational org member
	// See RFD for additional examples

	return false, nil
}

func (ghes *GitHubEventSource) respondToDeployRequest(approved bool, payload interface{}) error {
	// TODO call GH API

	return nil
}
