package record

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/gravitational/trace"
)

const RecordSchemaVersion = "v1"
const CanonicalMetaSchemaVersion = "v1"

// Common metadata for all records
type Common struct {
	ID                  string `json:"id"` // Generated from CanonicalMeta
	RecordSchemaVersion string `json:"record_schema_version"`
}

type Suite struct {
	Common

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
	Common

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

type Meta struct {
	Common
	CanonicalMeta
	GitMeta
	ActorMeta
	RunnerMeta
	Timestamp string `json:"timestamp"` // RFC3339 timestamp at creation
}

type GitMeta struct {
	GitRef     string `json:"git_ref,omitempty"`
	GitRefName string `json:"git_ref_name,omitempty"`
	BaseRef    string `json:"git_base_ref,omitempty"`
	HeadRef    string `json:"git_head_ref,omitempty"`
}

type ActorMeta struct {
	Actor   string `json:"actor_name,omitempty"`
	ActorID string `json:"actor_id,omitempty"`
}

type RunnerMeta struct {
	RunnerArch        string `json:"runner_arch,omitempty"`
	RunnerOS          string `json:"runner_os,omitempty"`
	RunnerName        string `json:"runner_name,omitempty"`
	RunnerEnvironment string `json:"runner_environment,omitempty"`
}

// Used to generate primary index
type CanonicalMeta struct {
	CanonicalMetaSchemaVersion string `json:"canonical_meta_schema_version"`
	Provider                   string `json:"provider"`
	RepositoryName             string `json:"repository_name"` // Human readable name of the repository
	Workflow                   string `json:"workflow"`
	Job                        string `json:"job"`
	RunID                      string `json:"run_id"`
	RunAttempt                 int    `json:"run_attempt"`
	SHA                        string `json:"git_sha"`
}

func (c CanonicalMeta) Id() (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", trace.Wrap(err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
