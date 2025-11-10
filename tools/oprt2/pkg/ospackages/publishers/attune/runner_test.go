/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package attune

import (
	"context"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/stretchr/testify/mock"
)

type mockRunner struct {
	mock.Mock
}

var _ commandrunner.Runner = (*mockRunner)(nil)

func (mr *mockRunner) Run(ctx context.Context, name string, args ...string) error {
	calledArgs := make([]any, 0, 2+len(args))
	calledArgs = append(calledArgs, ctx, name)
	for _, arg := range args {
		// Implicitly convert from `string` to `interface{}`
		calledArgs = append(calledArgs, arg)
	}

	return mr.Called(calledArgs...).Error(0)
}
