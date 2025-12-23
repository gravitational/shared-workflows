package github

import (
	"encoding/json"
	"fmt"
	"os"
)

// PullRequestEvent represents the structure of a GitHub Pull Request event payload.
// https://docs.github.com/en/webhooks/webhook-events-and-payloads#pull_request
type PullRequestEvent struct {
	PullRequest struct {
		State string `json:"state"`
	} `json:"pull_request"`
}

const (
	EnvGithubEventPath = "GITHUB_EVENT_PATH"
	PRStateOpen        = "open"
	PRStateClosed      = "closed"
)

func GetPullRequestEvent() (*PullRequestEvent, error) {
	eventPath := os.Getenv(EnvGithubEventPath)
	if eventPath == "" {
		return nil, fmt.Errorf("%s is not set", EnvGithubEventPath)
	}

	eventFile, err := os.Open(eventPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open GitHub event file: %w", err)
	}
	defer eventFile.Close()
	var event PullRequestEvent
	if err := json.NewDecoder(eventFile).Decode(&event); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub event JSON: %w", err)
	}

	return &event, nil
}
