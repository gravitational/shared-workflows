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

package github

// CheckStatus represents the status of a GitHub check run.
//
// GitHub check run statuses and conclusions are used by GitHub Actions and the Checks API.
// It's important to note that this may not be an exhaustive list as GitHub may introduce
// new statuses or conclusions in the future. For forwards compatibility, no validation is
// performed to ensure a value is one of the defined constants. Users of these types should
// handle unexpected values gracefully.
//
// See: https://docs.github.com/en/rest/checks/runs#check-statuses-and-conclusions
type CheckStatus string

const (
	// CheckStatusCompleted indicates the check run completed and has a conclusion.
	CheckStatusCompleted CheckStatus = "completed"
	// CheckStatusExpected indicates the check run is waiting for a status to be reported (GitHub Actions only).
	CheckStatusExpected CheckStatus = "expected"
	// CheckStatusFailure indicates the check run failed.
	CheckStatusFailure CheckStatus = "failure"
	// CheckStatusInProgress indicates the check run is in progress.
	CheckStatusInProgress CheckStatus = "in_progress"
	// CheckStatusPending indicates the check run is at the front of the queue but the group-based concurrency limit has been reached (GitHub Actions only).
	CheckStatusPending CheckStatus = "pending"
	// CheckStatusQueued indicates the check run has been queued.
	CheckStatusQueued CheckStatus = "queued"
	// CheckStatusRequested indicates the check run has been created but has not been queued (GitHub Actions only).
	CheckStatusRequested CheckStatus = "requested"
	// CheckStatusStartupFailure indicates the check suite failed during startup. Not applicable to check runs (GitHub Actions only).
	CheckStatusStartupFailure CheckStatus = "startup_failure"
	// CheckStatusWaiting indicates the check run is waiting for a deployment protection rule to be satisfied (GitHub Actions only).
	CheckStatusWaiting CheckStatus = "waiting"
)

// CheckConclusion represents the conclusion of a completed GitHub check run.
// See [CheckStatus] for more information about check run statuses and conclusions.
type CheckConclusion string

const (
	// CheckConclusionActionRequired indicates the check run provided required actions upon its completion.
	CheckConclusionActionRequired CheckConclusion = "action_required"
	// CheckConclusionCancelled indicates the check run was cancelled before it completed.
	CheckConclusionCancelled CheckConclusion = "cancelled"
	// CheckConclusionFailure indicates the check run failed.
	CheckConclusionFailure CheckConclusion = "failure"
	// CheckConclusionNeutral indicates the check run completed with a neutral result. Treated as success for dependent checks.
	CheckConclusionNeutral CheckConclusion = "neutral"
	// CheckConclusionSkipped indicates the check run was skipped. Treated as success for dependent checks.
	CheckConclusionSkipped CheckConclusion = "skipped"
	// CheckConclusionStale indicates the check run was marked stale by GitHub because it took too long.
	CheckConclusionStale CheckConclusion = "stale"
	// CheckConclusionSuccess indicates the check run completed successfully.
	CheckConclusionSuccess CheckConclusion = "success"
	// CheckConclusionTimedOut indicates the check run timed out.
	CheckConclusionTimedOut CheckConclusion = "timed_out"
)
