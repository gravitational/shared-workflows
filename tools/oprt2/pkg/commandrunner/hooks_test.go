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

package commandrunner

import (
	"context"
	"os/exec"

	"github.com/stretchr/testify/mock"
)

type mockHook struct {
	mock.Mock
}

var _ Hook = (*mockHook)(nil)

func (mh *mockHook) Name() string {
	return mh.Called().Get(0).(string)
}

func (mh *mockHook) Setup(ctx context.Context) error {
	return mh.Called(ctx).Error(0)
}

func (mh *mockHook) PreCommand(ctx context.Context, name *string, args *[]string) error {
	return mh.Called(ctx, name, args).Error(0)
}

func (mh *mockHook) Command(ctx context.Context, cmd *exec.Cmd) error {
	return mh.Called(ctx, cmd).Error(0)
}

func (mh *mockHook) Cleanup(ctx context.Context) error {
	return mh.Called(ctx).Error(0)
}
