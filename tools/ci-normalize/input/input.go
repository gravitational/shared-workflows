package input

import (
	"context"
)

// Producer defines an interface for producing records.
type Producer interface {
	Produce(ctx context.Context, write func(any) error) error
}

// PassthroughProducer is a Producer that emits a single record as-is.
type PassthroughProducer struct {
	record any
}

func NewPassthroughProducer(record any) *PassthroughProducer {
	return &PassthroughProducer{record: record}
}

func (p *PassthroughProducer) Produce(ctx context.Context, emit func(any) error) error {
	return emit(p.record)
}
