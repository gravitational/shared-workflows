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

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	"github.com/stretchr/testify/mock"
)

type mockFileManager struct {
	mock.Mock
}

var _ filemanager.FileManager = (*mockFileManager)(nil)

func (mfm *mockFileManager) ListItems(ctx context.Context) ([]string, error) {
	args := mfm.Called(ctx)
	return args.Get(0).([]string), args.Error(1)
}

func (mfm *mockFileManager) GetLocalFilePath(ctx context.Context, item string) (string, error) {
	args := mfm.Called(ctx, item)
	return args.Get(0).(string), args.Error(1)
}

func (mfm *mockFileManager) Name() string {
	return mfm.Called().Get(0).(string)
}

func (mfm *mockFileManager) Close() error {
	return mfm.Called().Error(0)
}
