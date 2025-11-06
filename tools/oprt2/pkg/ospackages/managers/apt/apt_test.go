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
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages/publishers/discard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPT(t *testing.T) {
	fileManager := &mockFileManager{}

	commonTests := func(t *testing.T, m *Manager) {
		require.NotNil(t, m)
		assert.Equal(t, logging.DiscardLogger, m.logger)
		assert.Equal(t, discard.DiscardPublisher, m.publisher)
		assert.Equal(t, fileManager, m.fileManager)
	}

	t.Run("basic manager", func(t *testing.T) {
		commonTests(t, NewManager(fileManager))
	})

	t.Run("runner with options", func(t *testing.T) {
		calledA := false
		optionA := func(m *Manager) {
			calledA = true
			assert.NotNil(t, m)
		}

		calledB := false
		optionB := func(m *Manager) {
			calledB = true
			assert.True(t, calledA) // Check call order
			assert.NotNil(t, m)
		}

		r := NewManager(fileManager, optionA, optionB)

		commonTests(t, r)
		assert.True(t, calledB)
	})
}

func TestGetPackagePublishingTasks(t *testing.T) {
	filenameMatcherA := regexp.MustCompile(`^file A\.deb$`)
	filenameMatcherB := regexp.MustCompile(`^file B\.deb$`)

	providedRepos := map[string]map[string]map[string][]*regexp.Regexp{
		"repo a": {
			"distribution a": {
				"component a": {
					filenameMatcherA,
					filenameMatcherA,
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
				},
			},
			"distribution b": {
				"component a": {
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
					filenameMatcherA,
					filenameMatcherB,
				},
			},
		},
		"repo b": {
			"distribution a": {
				"component a": {
					filenameMatcherA,
					filenameMatcherA,
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
				},
			},
			"distribution b": {
				"component a": {
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
					filenameMatcherA,
					filenameMatcherB,
				},
			},
		},
	}

	ctx := t.Context()

	fileManager := &mockFileManager{}
	fileManager.On("ListItems", ctx).Return([]string{"file A.deb", "file B.deb", "other/file.deb"}, nil)
	fileManager.On("GetLocalFilePath", ctx, "file A.deb").Return(filepath.Join("test dir", "file A.deb"), nil)
	fileManager.On("GetLocalFilePath", ctx, "file B.deb").Return(filepath.Join("test dir", "file B.deb"), nil)
	fileManager.On("GetLocalFilePath", ctx, "other/file.deb").Return("", "this should not be called").Maybe()

	publisher := &mockAPTPublisher{}
	publisher.On("PublishToAPTRepo", ctx, "repo a", "distribution a", "component a", "test dir/file A.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo a", "distribution a", "component a", "test dir/file B.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo a", "distribution a", "component b", "test dir/file A.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo a", "distribution b", "component a", "test dir/file B.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo a", "distribution b", "component b", "test dir/file A.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo a", "distribution b", "component b", "test dir/file B.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo b", "distribution a", "component a", "test dir/file A.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo b", "distribution a", "component a", "test dir/file B.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo b", "distribution a", "component b", "test dir/file A.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo b", "distribution b", "component a", "test dir/file B.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo b", "distribution b", "component b", "test dir/file A.deb").Return(nil)
	publisher.On("PublishToAPTRepo", ctx, "repo b", "distribution b", "component b", "test dir/file B.deb").Return(nil)

	manager := NewManager(fileManager, WithRepos(providedRepos), WithPublisher(publisher))

	tasks, err := manager.GetPackagePublishingTasks(ctx)

	assert.NoError(t, err)
	assert.Len(t, tasks, 12)

	for _, task := range tasks {
		assert.NoError(t, task(ctx))
	}
}

func TestName(t *testing.T) {
	assert.NotEmpty(t, NewManager(nil).Name())
}

func TestClose(t *testing.T) {
	assert.NoError(t, NewManager(nil).Close(t.Context()))
}
