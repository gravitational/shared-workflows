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
	"os/exec"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/stretchr/testify/mock"
)

type mockHook struct {
	mock.Mock
}

var _ commandrunner.Hook = (*mockHook)(nil)

func (mh *mockHook) Name() string {
	return mh.Called().Get(0).(string)
}

func (mh *mockHook) Command(ctx context.Context, cmd *exec.Cmd) error {
	return mh.Called(ctx, cmd).Error(0)
}

func (mh *mockHook) Close(ctx context.Context) error {
	return mh.Called(ctx).Error(0)
}
