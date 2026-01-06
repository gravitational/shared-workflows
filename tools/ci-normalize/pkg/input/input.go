package input

import (
	"context"
)

type Producer interface {
	Produce(ctx context.Context, write func(any) error) error
}

type PassthroughProducer struct {
	record any
}

func NewPassthroughProducer(record any) *PassthroughProducer {
	return &PassthroughProducer{record: record}
}

func (p *PassthroughProducer) Produce(ctx context.Context, emit func(any) error) error {
	return emit(p.record)
}
