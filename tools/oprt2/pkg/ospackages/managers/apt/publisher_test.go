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

package apt

import (
	"context"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages"
	"github.com/stretchr/testify/mock"
)

type mockAPTPublisher struct {
	mock.Mock
}

var _ ospackages.APTPublisher = (*mockAPTPublisher)(nil)

func (maptp *mockAPTPublisher) Name() string {
	return maptp.Called().Get(0).(string)
}

func (maptp *mockAPTPublisher) Close(ctx context.Context) error {
	return maptp.Called(ctx).Error(0)
}

func (maptp *mockAPTPublisher) PublishToAPTRepo(ctx context.Context, repo, distro, component, packageFilePath string) error {
	return maptp.Called(ctx, repo, distro, component, packageFilePath).Error(0)
}
