/*
Copyright 2023 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package env

// DefaultApproverCount is the default number of required approvers.
// This value should be greater than 1.
const DefaultApproverCount = 2

// Changes contains classification of the PR changes.
type Changes struct {
	// Code indicates the PR contains code changes.
	Code bool
	// Docs indicates the PR contains docs changes.
	Docs bool
	// Release indicates a release PR.
	Release bool
	// Large indicates the PR changeset is large.
	Large bool
	// Number of required approvers (default is DefaultApproverCount)
	ApproverCount int
}
