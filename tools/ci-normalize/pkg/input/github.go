package input

import (
	"context"
	"strings"
	"time"

	env "github.com/caarlos0/env/v9"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/gravitational/trace"
)

type GithubMetaProducer struct{}

func NewGithubMetaProducer() *GithubMetaProducer {
	return &GithubMetaProducer{}
}

// GithubMeta is the envrioment read from github, this should before the test job to capture the metadata.
// Currently only supports reading from env, could consider reading from env file in the future.
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
	BaseRef           string `env:"GITHUB_BASE_REF"`
	HeadRef           string `env:"GITHUB_HEAD_REF"`
	GithubActor       string `env:"GITHUB_ACTOR"`
	GithubActorID     string `env:"GITHUB_ACTOR_ID"`
	RunnerArch        string `env:"RUNNER_ARCH"`
	RunnerOS          string `env:"RUNNER_OS"`
	RunnerName        string `env:"RUNNER_NAME"`
	RunnerEnvironment string `env:"RUNNER_ENVIRONMENT"`
}

func metaFromGh(gh GithubMeta) (*record.Meta, error) {
	now := time.Now()
	canonical := record.CanonicalMeta{
		CanonicalMetaSchemaVersion: record.CanonicalMetaSchemaVersion,
		Provider:                   "github.com",
		Repository:                 strings.ToLower(gh.Repository),
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
		Common: record.Common{
			ID:                  id,
			RecordSchemaVersion: record.RecordSchemaVersion,
		},
		CanonicalMeta: canonical,
		GitMeta: record.GitMeta{
			GitRef:  gh.GitRef,
			BaseRef: gh.BaseRef,
			HeadRef: gh.HeadRef,
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

func (p *GithubMetaProducer) Produce(ctx context.Context, emit func(any) error) error {
	var ghMeta GithubMeta
	if err := env.Parse(&ghMeta); err != nil {
		return trace.Wrap(err)
	}

	meta, err := metaFromGh(ghMeta)
	if err != nil {
		return trace.Wrap(err)
	}

	if err = emit(meta); err != nil {
		return trace.Wrap(err)
	}

	return nil
}
