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

package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"testing"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
)

func TestParseFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		expected  flags
		errAssert require.ErrorAssertionFunc
	}{
		{
			name: "parse check",
			args: []string{
				"-workflow=check",
				"-token=token",
				fmt.Sprintf("-reviewers=%s", base64.StdEncoding.EncodeToString([]byte("reviewers"))),
			},
			expected: flags{
				workflow:  "check",
				token:     "token",
				reviewers: "reviewers",
			},
			errAssert: require.NoError,
		},
		{
			name: "parse assign",
			args: []string{
				"-workflow=assign",
				"-token=token",
				fmt.Sprintf("-reviewers=%s", base64.StdEncoding.EncodeToString([]byte("reviewers"))),
			},
			expected: flags{
				workflow:  "assign",
				token:     "token",
				reviewers: "reviewers",
			},
			errAssert: require.NoError,
		},
		{
			name: "reviewers not present for check",
			args: []string{
				"-workflow=check",
				"-token=token",
			},
			errAssert: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorIs(t, err, trace.BadParameter("reviewers required for assign and check"))
			},
		},
		{
			name: "reviewers not present for assign",
			args: []string{
				"-workflow=assign",
				"-token=token",
			},
			errAssert: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorIs(t, err, trace.BadParameter("reviewers required for assign and check"))
			},
		},
		{
			name: "reviewers not present for check (local)",
			args: []string{
				"-workflow=check",
				"-token=token",
				"-local=true",
			},
			expected: flags{
				workflow: "check",
				token:    "token",
				local:    true,
			},
			errAssert: require.NoError,
		},
		{
			name: "reviewers not present for label",
			args: []string{
				"-workflow=label",
				"-token=token",
			},
			expected: flags{
				workflow: "label",
				token:    "token",
			},
			errAssert: require.NoError,
		},
		{
			name: "reviewers ignored for label",
			args: []string{
				"-workflow=label",
				"-token=token",
				"-reviewers=reviewers",
			},
			expected: flags{
				workflow: "label",
				token:    "token",
			},
			errAssert: require.NoError,
		},
		{
			name: "no workflow",
			args: []string{
				"-token=token",
			},
			errAssert: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorIs(t, err, trace.BadParameter("workflow missing"))
			},
		},
		{
			name: "no token",
			args: []string{
				"-workflow=check",
			},
			errAssert: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorIs(t, err, trace.BadParameter("token missing"))
			},
		},
		{
			name: "reviewers not parseable",
			args: []string{
				"-workflow=check",
				"-token=token",
				"-reviewers=reviewers",
			},
			errAssert: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "illegal base64 data at input byte 8")
			},
		},
		{
			name: "parse exits with help",
			args: []string{
				"-help",
			},
			errAssert: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorIs(t, err, flag.ErrHelp)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flags, err := parseFlags(test.args)
			require.Equal(t, test.expected, flags)
			test.errAssert(t, err)
		})
	}
}
