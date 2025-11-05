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

package discard

import (
	"context"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages"
)

// DiscardPublisher is a publisher that does nothing.
type DiscardPublisher struct{}

var _ ospackages.APTPublisher = (*DiscardPublisher)(nil)

// NewDiscardPublisher creates a new, do-nothing publisher.
func NewDiscardPublisher() *DiscardPublisher {
	return &DiscardPublisher{}
}

// Name is the name of the package publisher.
func (*DiscardPublisher) Name() string {
	return "discard"
}

// PublishToAPTRepo publishes the package at the provided file path to the publisher's APT repo,
// with the set distro, and component.
func (*DiscardPublisher) PublishToAPTRepo(ctx context.Context, repo, distro, component, packageFileName string) error {
	return nil
}

// Close closes the publisher
func (*DiscardPublisher) Close(_ context.Context) error {
	return nil
}
