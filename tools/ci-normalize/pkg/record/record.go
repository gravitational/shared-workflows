package record

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/gravitational/trace"
)

type Common struct {
	Repo         string `json:"repo"`        // env:GITHUB_REPOSITORY
	RunID        string `json:"run_id"`      // env:GITHUB_RUN_ID
	RunNumber    int    `json:"run_number"`  // env:GITHUB_RUN_NUMBER
	RunAttempt   int    `json:"run_attempt"` // env:GITHUB_RUN_ATTEMPT
	JobName      string `json:"job_name"`    // env:GITHUB_JOB
	WorkflowName string `json:"workflow"`    // env:GITHUB_WORKFLOW
}

func (c Common) RunKey() (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", trace.Wrap(err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func NewCommonFromMap(env map[string]string) Common {
	runNumber, _ := strconv.Atoi(env["GITHUB_RUN_NUMBER"])
	runAttempt, _ := strconv.Atoi(env["GITHUB_RUN_ATTEMPT"])

	return Common{
		Repo:         env["GITHUB_REPOSITORY"],
		RunID:        env["GITHUB_RUN_ID"],
		RunNumber:    runNumber,
		RunAttempt:   runAttempt,
		JobName:      env["GITHUB_JOB"],
		WorkflowName: env["GITHUB_WORKFLOW"],
	}
}

type Suite struct {
	Common        // Common metadata for all records
	RunId  string `json:"run_id"`

	Name       string            `json:"suite_name"`
	Timestamp  string            `json:"timestamp"` // RFC3339
	Tests      int               `json:"tests"`
	Failures   int               `json:"failures"`
	Errors     int               `json:"errors"`
	Skipped    int               `json:"skipped"`
	DurationMs int64             `json:"duration_ms"`
	Properties map[string]string `json:"properties,omitempty"`
}

// TestcaseInfo holds metadata about the individual test case
type Testcase struct {
	Common        // Common metadata for all records
	RunId  string `json:"run_id"`

	Name       string `json:"test_name"`
	SuiteName  string `json:"suite_name"`
	Classname  string `json:"classname"` // When using gosumtest this is the same as suite_name
	DurationMs int64  `json:"duration_ms"`
	Status     string `json:"status"` // "passed", "failed", "skipped", "error"

	// Flattened message, if need this cardinality in the future other fields can be added.
	SkipMessage    string `json:"skip_message,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	FailureMessage string `json:"failure_message,omitempty"`
}

type Job struct {
	Common
	RunId string `json:"run_id"`

	// git stuff
	CommitSHA string `json:"commit_sha"`         // env:GITHUB_SHA
	GitRef    string `json:"git_ref"`            // env:GITHUB_REF
	BaseRef   string `json:"base_ref,omitempty"` // env:GITHUB_BASE_REF
	HeadRef   string `json:"head_ref,omitempty"` // env:GITHUB_HEAD_REF

	// github
	GithubActor   string `json:"actor,omitempty"`    // env:GITHUB_ACTOR
	GithubActorID string `json:"actor_id,omitempty"` // env:GITHUB_ACTOR_ID

	// runner stuff
	RunnerArch        string `json:"runner_arch,omitempty"` // env:RUNNER_ARCH
	RunnerOS          string `json:"runner_os,omitempty"`   // env:RUNNER_OS
	RunnerName        string `json:"runner_name,omitempty"` // env:RUNNER_NAME
	RunnerEnvironment string `json:"runner_environment"`    // env:RUNNER_ENVIRONMENT

	// TODO: figure out what other metadata to include
}

func NewJobFromMap(env map[string]string) (*Job, error) {
	common := NewCommonFromMap(env)

	id, err := common.RunKey()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &Job{
		Common: common,
		RunId:  id,

		// git info
		CommitSHA: env["GITHUB_SHA"],
		GitRef:    env["GITHUB_REF"],
		BaseRef:   env["GITHUB_BASE_REF"],
		HeadRef:   env["GITHUB_HEAD_REF"],

		// github actor info
		GithubActor:   env["GITHUB_ACTOR"],
		GithubActorID: env["GITHUB_ACTOR_ID"],

		// runner info
		RunnerArch:        env["RUNNER_ARCH"],
		RunnerOS:          env["RUNNER_OS"],
		RunnerName:        env["RUNNER_NAME"],
		RunnerEnvironment: env["RUNNER_ENVIRONMENT"],
	}, nil
}
