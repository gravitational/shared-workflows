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

package meta

import (
	"strings"
	"time"

	"github.com/caarlos0/env/v9"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
	"github.com/gravitational/trace"
)

// GithubMeta holds GitHub Actions environment metadata.
type GithubMeta struct {
	// Required for canonical ids:
	Repository   string `env:"GITHUB_REPOSITORY,notEmpty"`
	RunID        string `env:"GITHUB_RUN_ID,notEmpty"`
	RunNumber    int    `env:"GITHUB_RUN_NUMBER,notEmpty"`
	RunAttempt   int    `env:"GITHUB_RUN_ATTEMPT,notEmpty"`
	JobName      string `env:"GITHUB_JOB,notEmpty"`
	WorkflowName string `env:"GITHUB_WORKFLOW,notEmpty"`
	GitSHA       string `env:"GITHUB_SHA,notEmpty"`

	GitRef            string `env:"GITHUB_REF"`
	GitRefName        string `env:"GITHUB_REF_NAME"`
	BaseRef           string `env:"GITHUB_BASE_REF"`
	HeadRef           string `env:"GITHUB_HEAD_REF"`
	GithubActor       string `env:"GITHUB_ACTOR"`
	GithubActorID     string `env:"GITHUB_ACTOR_ID"`
	RunnerArch        string `env:"RUNNER_ARCH"`
	RunnerOS          string `env:"RUNNER_OS"`
	RunnerName        string `env:"RUNNER_NAME"`
	RunnerEnvironment string `env:"RUNNER_ENVIRONMENT"`
}

// newFromGithubEnv reads metadata from GitHub Actions environment variables and maps them to a [record.Meta].
func newFromGithubEnv() (*record.Meta, error) {
	now := time.Now()
	var gh GithubMeta
	if err := env.Parse(&gh); err != nil {
		return nil, trace.Wrap(err, "could not read Github metadata from env")
	}

	canonical := record.CanonicalMeta{
		CanonicalMetaSchemaVersion: record.CanonicalMetaSchemaVersion,
		Provider:                   "github.com",
		RepositoryName:             strings.ToLower(gh.Repository),
		Workflow:                   strings.ToLower(gh.WorkflowName),
		Job:                        strings.ToLower(gh.JobName),
		RunID:                      gh.RunID,
		RunAttempt:                 gh.RunAttempt,
		SHA:                        strings.ToLower(gh.GitSHA),
	}

	id, err := canonical.Id()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &record.Meta{
		MetaID:              id,
		RecordSchemaVersion: record.RecordSchemaVersion,
		CanonicalMeta:       canonical,
		GitMeta: record.GitMeta{
			GitRef:     gh.GitRef,
			GitRefName: gh.GitRefName,
			BaseRef:    gh.BaseRef,
			HeadRef:    gh.HeadRef,
		},
		ActorMeta: record.ActorMeta{
			Actor:   gh.GithubActor,
			ActorID: gh.GithubActorID,
		},
		RunnerMeta: record.RunnerMeta{
			RunnerArch:        gh.RunnerArch,
			RunnerOS:          gh.RunnerOS,
			RunnerName:        gh.RunnerName,
			RunnerEnvironment: gh.RunnerEnvironment,
		},
		Timestamp: now.Format(time.RFC3339),
	}, nil
}
