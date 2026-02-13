// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package record

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/gravitational/trace"
)

const RecordSchemaVersion = "v2"
const CanonicalMetaSchemaVersion = "v1"

// Suite represents a test suite containing multiple test cases.
type Suite struct {
	SuiteID             string `json:"suite_id"` // SHA256(meta.id + name)
	MetaID              string `json:"meta_id"`
	RecordSchemaVersion string `json:"record_schema_version"`

	Name       string            `json:"suite_name"`
	Timestamp  string            `json:"timestamp"` // RFC3339
	Tests      int               `json:"tests"`
	Failures   int               `json:"failures"`
	Errors     int               `json:"errors"`
	Skipped    int               `json:"skipped"`
	DurationMs int64             `json:"duration_ms"`
	Properties map[string]string `json:"properties,omitempty"`
}

// GetId is a nil safe getter for suite id
func (s *Suite) GetId() string {
	if s != nil {
		return s.SuiteID
	}
	return ""
}

// GetName is a nil safe getter for suite name
func (s *Suite) GetName() string {
	if s != nil {
		return s.Name
	}
	return ""
}

// Testcase represents a single test case within a test suite.
type Testcase struct {
	TestcaseID          string `json:"testcase_id"` // SHA256(suite.id + name)
	SuiteID             string `json:"suite_id"`
	MetaID              string `json:"meta_id"`
	RecordSchemaVersion string `json:"record_schema_version"`

	Name       string `json:"test_name"`
	SuiteName  string `json:"suite_name"`
	Classname  string `json:"classname"` // When using gosumtest this is the same as suite_name
	DurationMs int64  `json:"duration_ms"`
	Status     string `json:"status"` // "pass", "failed", "skipped", "error"

	// Flattened message, if need this cardinality in the future other fields can be added.
	SkipMessage    string `json:"skip_message,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	FailureMessage string `json:"failure_message,omitempty"`
}

// Meta holds metadata about the CI workflow run.
type Meta struct {
	MetaID              string `json:"meta_id"` // Generated from CanonicalMeta
	RecordSchemaVersion string `json:"record_schema_version"`

	CanonicalMeta
	GitMeta
	ActorMeta
	RunnerMeta
	Timestamp string `json:"timestamp"` // RFC3339 timestamp at creation
}

// GetId is a nil safe getter for meta id
func (m *Meta) GetId() string {
	if m != nil {
		return m.MetaID
	}
	return ""
}

// GitMeta holds Git-related metadata.
type GitMeta struct {
	GitRef     string `json:"git_ref,omitempty"`
	GitRefName string `json:"git_ref_name,omitempty"`
	BaseRef    string `json:"git_base_ref,omitempty"`
	HeadRef    string `json:"git_head_ref,omitempty"`
}

// ActorMeta holds information about the actor who triggered the workflow.
type ActorMeta struct {
	Actor   string `json:"actor_name,omitempty"`
	ActorID string `json:"actor_id,omitempty"`
}

// RunnerMeta holds information about the CI runner environment.
type RunnerMeta struct {
	RunnerArch        string `json:"runner_arch,omitempty"`
	RunnerOS          string `json:"runner_os,omitempty"`
	RunnerName        string `json:"runner_name,omitempty"`
	RunnerEnvironment string `json:"runner_environment,omitempty"`
}

// CanonicalMeta holds information used to generate a unique ID for the CI workflow run.
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

// Id generates a unique identifier for the CanonicalMeta.
func (c CanonicalMeta) Id() (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", trace.Wrap(err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

type Writer interface {
	WriteSuite(*Suite) error
	WriteTestcase(*Testcase) error
	WriteMeta(*Meta) error
}
