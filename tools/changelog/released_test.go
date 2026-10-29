/*
Copyright 2024 Gravitational, Inc.

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

package main

import (
	"testing"

	"github.com/gravitational/shared-workflows/libs/git"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeSubject(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		want    string
	}{
		{
			name:    "trailing PR ref is dropped",
			subject: "Fix the thing (#101)",
			want:    "Fix the thing",
		},
		{
			name: "every trailing PR ref is dropped",
			// A backport of #100 merged as #200. The release branch may carry
			// one form and the base branch the other, so both have to reduce
			// to the same key.
			subject: "Fix the thing (#100) (#200)",
			want:    "Fix the thing",
		},
		{
			name:    "squash merge bullet is dropped",
			subject: "* Fix the thing (#101)",
			want:    "Fix the thing",
		},
		{
			name:    "dash bullet is dropped",
			subject: "- Fix the thing",
			want:    "Fix the thing",
		},
		{
			name:    "whitespace is collapsed and trimmed",
			subject: "  Fix   the\tthing  ",
			want:    "Fix the thing",
		},
		{
			name:    "mid-subject PR ref is kept",
			subject: "Fix the thing (#101) and another",
			want:    "Fix the thing (#101) and another",
		},
		{
			name:    "subject without a PR ref is unchanged",
			subject: "Update the Go toolchain to 1.26.10",
			want:    "Update the Go toolchain to 1.26.10",
		},
		{
			name:    "trailing colon is kept",
			subject: "Minimize Kube Agent permissions:",
			want:    "Minimize Kube Agent permissions:",
		},
		{
			name:    "empty",
			subject: "",
			want:    "",
		},
		{
			name:    "bullet alone normalizes to empty",
			subject: "* ",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeSubject(tt.subject))
		})
	}
}

func TestReleasedSetSuppressed(t *testing.T) {
	released := &releasedSet{
		prNumbers: map[int]struct{}{2633: {}},
		subjects: map[string]struct{}{
			"Update electron to 43.8.1":                                  {},
			"fix: Bound PROXY v1 header parsing to prevent pre-auth DoS": {},
		},
	}

	candidates := []git.Commit{
		{SHA: "aaa", Subject: "Make legacy resource adapters return the same type (#2633)"},
		{SHA: "bbb", Subject: "Update electron to 43.8.1 (#2914)"},
		{SHA: "ccc", Subject: "fix: Bound PROXY v1 header parsing to prevent pre-auth DoS (#1686)"},
		{SHA: "ddd", Subject: "Genuinely new change (#3001)"},
		{SHA: "eee", Subject: "New change with no PR ref"},
	}

	keep, dropped := released.suppressed(candidates)

	assert.Equal(t, []int{3001}, prNumbersOf(keep))
	assert.Len(t, keep, 2)

	gotReasons := make(map[string]string, len(dropped))
	for _, suppression := range dropped {
		gotReasons[suppression.Commit.SHA] = suppression.Reason
	}
	assert.Equal(t, map[string]string{
		"aaa": "pull request already released",
		"bbb": "subject already released",
		"ccc": "subject already released",
	}, gotReasons)
}

func TestReleasedSetSuppressed_EmptySetKeepsEverything(t *testing.T) {
	// When the base tag is an ancestor of the head branch the released set is
	// empty, and the tool must behave exactly as it did before.
	released := &releasedSet{
		prNumbers: map[int]struct{}{},
		subjects:  map[string]struct{}{},
	}

	candidates := []git.Commit{
		{SHA: "aaa", Subject: "Fix one thing (#101)"},
		{SHA: "bbb", Subject: "Fix another thing (#102)"},
	}

	keep, dropped := released.suppressed(candidates)
	assert.Equal(t, candidates, keep)
	assert.Empty(t, dropped)
}

func TestReleasedSetSuppressed_ForeignPRNumberIsNotTrusted(t *testing.T) {
	// A squash merge can carry commits authored in a private fork, whose
	// "(#N)" suffixes number that fork's pull requests. Those arrive as
	// subjects only, so a pull request of this repo that happens to share the
	// number must survive.
	released := &releasedSet{
		prNumbers: map[int]struct{}{},
		subjects: map[string]struct{}{
			"[v18] Fix panic on requests with nil payload": {},
		},
	}

	candidates := []git.Commit{
		{SHA: "aaa", Subject: "[v18] MCP OAuth app settings and tsh mcp login fixes (#2700)"},
		{SHA: "bbb", Subject: "[v18] Fix panic on requests with nil payload (#2700) (#227)"},
	}

	keep, dropped := released.suppressed(candidates)

	// The real PR #2700 is kept...
	assert.Equal(t, []int{2700}, prNumbersOf(keep))
	assert.Equal(t, "aaa", keep[0].SHA)
	// ...while the private-fork commit that shares the number is suppressed on
	// its subject, which requires stripping both trailing refs.
	assert.Len(t, dropped, 1)
	assert.Equal(t, "bbb", dropped[0].Commit.SHA)
}

func TestPRNumbersOf(t *testing.T) {
	commits := []git.Commit{
		{Subject: "Fix one thing (#103)"},
		{Subject: "chore: no PR reference"},
		{Subject: "Fix another thing (#101)"},
		{Subject: "Backport note (#103)"},
	}

	// Order preserved, duplicates collapsed, unreferenced commits skipped.
	assert.Equal(t, []int{103, 101}, prNumbersOf(commits))
	assert.Nil(t, prNumbersOf(nil))
}
