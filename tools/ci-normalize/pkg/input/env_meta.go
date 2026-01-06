package input

import (
	"context"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/gravitational/trace"
)

type GithubMetaProducer struct {
	envMap map[string]string
}

func NewGithubMetaProducer(envMap map[string]string) *GithubMetaProducer {
	return &GithubMetaProducer{
		envMap: envMap,
	}
}

func (p *GithubMetaProducer) Produce(ctx context.Context, emit func(any) error) error {
	meta, err := record.NewJobFromMap(p.envMap)
	if err != nil {
		return trace.Wrap(err)
	}

	err = emit(meta)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}
