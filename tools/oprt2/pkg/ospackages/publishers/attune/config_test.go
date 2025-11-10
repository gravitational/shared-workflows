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
	"testing"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/stretchr/testify/assert"
)

func Test_publisherFromConfig_Close(t *testing.T) {
	mockHookA := &mockHook{}
	mockHookB := &mockHook{}

	publisher := publisherFromConfig{
		hooks: []commandrunner.Hook{
			mockHookA,
			mockHookB,
		},
	}

	// Cancel the context to ensure that these are called even with a cancelled context.
	// This is critical to reduce the chance of leaked secrets.
	cancelledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	mockHookA.On("Close", cancelledCtx).Return(assert.AnError).Once()
	mockHookA.On("Name").Return("test hook A").Maybe()
	mockHookB.On("Close", cancelledCtx).Return(nil).Once()
	mockHookB.On("Name").Return("test hook B").Maybe()

	err := publisher.Close(cancelledCtx)
	assert.Error(t, err)
}
