package notarize

import (
	"encoding/json"
	"log/slog"

	"github.com/gravitational/trace"
)

// SubmissionResponseData contains information about the status of a submission.
//
// Reference: https://developer.apple.com/documentation/notaryapi/submissionresponse/data-data.dictionary
type submissionResponseData struct {
	ID      string `json:"id,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
	Status  string `json:"status,omitempty"`
}

// SubmitAndWait is a convenience function that wraps the Submit and WaitForSubmission functions.
func (t *Tool) SubmitAndWait(pathToPackage string) error {
	submissionID, err := t.Submit(pathToPackage)
	if err != nil {
		return trace.Wrap(err)
	}
	if err := t.WaitForSubmission(submissionID); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// Submit will submit a package for notarization.
// A package must be in the format of a zip, package installer (.pkg), or a disk image (.dmg)
// A success will return a submission ID which can be polled on later.
func (t *Tool) Submit(pathToPackage string) (id string, err error) {
	args := []string{
		"notarytool",
		"submit", pathToPackage,
		"--team-id", t.Creds.TeamID,
		"--apple-id", t.Creds.AppleUsername,
		"--password", t.Creds.ApplePassword,
		"--output-format", "json",
	}

	t.log.Info("submitting package for notarization", "package", pathToPackage)
	stdout, err := t.runRetryable(func() ([]byte, error) {
		stdout, err := t.cmdRunner.RunCommand("xcrun", args...)
		if err != nil {
			t.log.Error("submission error", "error", err)
		}
		return stdout, err
	})

	if err != nil {
		return "", trace.Wrap(err, "failed to submit package for notarization for %d attempts", t.maxRetries)
	}

	if t.dryRun { // If dry run, return a fake submission ID
		return "0", nil
	}

	var sub submissionResponseData
	if err := json.Unmarshal([]byte(stdout), &sub); err != nil {
		return "", trace.Wrap(err, "failed to parse output from submission request")
	}
	t.log.Info("submission successful", "response", sub)

	return sub.ID, nil
}

// WaitForSubmission waits for the submission process to be complete
func (t *Tool) WaitForSubmission(id string) error {
	args := []string{
		"notarytool",
		"wait", id,
		"--team-id", t.Creds.TeamID,
		"--apple-id", t.Creds.AppleUsername,
		"--password", t.Creds.ApplePassword,
		"--output-format", "json",
	}
	t.log.Info("waiting for submission to finish processing", "id", id)
	stdout, err := t.cmdRunner.RunCommand("xcrun", args...)
	if err != nil {
		return trace.Wrap(err, "failed while waiting for submission to finish processing")
	}

	var sub submissionResponseData
	if err := json.Unmarshal([]byte(stdout), &sub); err != nil {
		return trace.Wrap(err, "failed to parse output from submission request")
	}
	t.log.Info("waiting done", "response", sub)

	return nil
}

func (s submissionResponseData) LogValue() slog.Value {
	values := []slog.Attr{
		slog.String("id", s.ID),
	}
	if s.Path != "" {
		values = append(values, slog.String("path", s.Path))
	}
	if s.Message != "" {
		values = append(values, slog.String("message", s.Message))
	}
	if s.Status != "" {
		values = append(values, slog.String("status", s.Status))
	}
	return slog.GroupValue(values...)
}
