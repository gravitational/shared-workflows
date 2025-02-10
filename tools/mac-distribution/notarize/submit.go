package notarize

import (
	"encoding/json"

	"github.com/gravitational/trace"
)

// SubmissionResponseData contains information about the status of a submission.
//
// Reference: https://developer.apple.com/documentation/notaryapi/submissionresponse/data-data.dictionary
type submissionResponseData struct {
	ID string `json:"id,omitempty"`
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
		"--team-id", t.Creds.BundleID,
		"--apple-id", t.Creds.AppleUsername,
		"--apple-password", t.Creds.ApplePassword,
		"--output-format", "json",
	}

	stdout, err := t.cmdRunner.RunCommand("xcrun", args...)
	for i := 0; err != nil && i < t.retry; i += 1 {
		t.log.Error("submission error", "error", err)
		t.log.Info("retrying submission", "count", i+1)
		stdout, err = t.cmdRunner.RunCommand("xcrun", args...)
	}

	if err != nil {
		return "", trace.Wrap(err, "failed to submit package for notarization for %d attempts", t.retry)
	}

	if t.dryRun { // If dry run, return a fake submission ID
		return "0", nil
	}

	var sub submissionResponseData
	if err := json.Unmarshal([]byte(stdout), &sub); err != nil {
		return "", trace.Wrap(err, "failed to parse output from submission request")
	}

	return sub.ID, nil
}

// WaitForSubmission waits for the submission process to be complete
func (t *Tool) WaitForSubmission(id string) error {
	args := []string{
		"notarytool",
		"wait", id,
		"--team-id", t.Creds.BundleID,
		"--apple-id", t.Creds.AppleUsername,
		"--apple-password", t.Creds.ApplePassword,
		"--output-format", "json",
	}
	stdout, err := t.cmdRunner.RunCommand("xcrun", args...)
	if err != nil {
		return trace.Wrap(err, "failed while waiting for submission to finish processing")
	}
	t.log.Info("waiting done", "stdout", stdout)

	return nil
}
