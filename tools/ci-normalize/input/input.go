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
