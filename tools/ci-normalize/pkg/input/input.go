package input

import (
	"context"
)

type Producer interface {
	Produce(ctx context.Context, emit func(any) error) error
}
