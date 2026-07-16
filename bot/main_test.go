package main

import (
	"encoding/base64"
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlagsReviewersBase64(t *testing.T) {
	dummyArgs := []string{
		os.Args[0],
		"-workflow",
		"dummy-value",
		"-token",
		"dummy-value",
	}

	flagName := "-reviewers"
	testValueJSON := `{"key": "value"}`
	testValueBase64 := base64.StdEncoding.EncodeToString([]byte(testValueJSON))

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError) // Reset the flag lib
	os.Args = append(dummyArgs, flagName, testValueBase64)
	b64Flags, err := parseFlags()
	require.NoError(t, err)

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError) // Reset the flag lib
	os.Args = append(dummyArgs, flagName, testValueJSON)
	plainTextFlags, err := parseFlags()
	require.NoError(t, err)

	// Verify that the results are the same regardless of whether or not the reviewers
	// flag is base64 encoded
	assert.Equal(t, b64Flags.reviewers, plainTextFlags.reviewers)

	// Basic checks
	assert.Equal(t, testValueJSON, b64Flags.reviewers)
}

func TestParseFlagsWorkflowNeedsReviewers(t *testing.T) {
	const dummyToken = "test-token"
	const dummyReviewers = `{"core":{}}`

	tests := []struct {
		desc          string
		workflow      string
		reviewers     string
		local         bool
		wantErr       bool
		wantReviewers string
	}{
		// missing workflow
		{
			desc:    "missing workflow",
			wantErr: true,
		},
		// workflows that need reviewers – without reviewers → error
		{
			desc:     "assign without reviewers",
			workflow: "assign",
			wantErr:  true,
		},
		{
			desc:     "backport without reviewers",
			workflow: "backport",
			wantErr:  true,
		},
		{
			desc:     "bloat without reviewers",
			workflow: "bloat",
			wantErr:  true,
		},
		{
			desc:     "check without reviewers",
			workflow: "check",
			wantErr:  true,
		},
		{
			desc:     "exclude-flakes without reviewers",
			workflow: "exclude-flakes",
			wantErr:  true,
		},
		{
			desc:     "rfd without reviewers",
			workflow: "rfd",
			wantErr:  true,
		},
		// workflows that need reviewers – with reviewers → success
		{
			desc:          "assign with reviewers",
			workflow:      "assign",
			reviewers:     dummyReviewers,
			wantReviewers: dummyReviewers,
		},
		{
			desc:          "backport with reviewers",
			workflow:      "backport",
			reviewers:     dummyReviewers,
			wantReviewers: dummyReviewers,
		},
		{
			desc:          "bloat with reviewers",
			workflow:      "bloat",
			reviewers:     dummyReviewers,
			wantReviewers: dummyReviewers,
		},
		{
			desc:          "check with reviewers",
			workflow:      "check",
			reviewers:     dummyReviewers,
			wantReviewers: dummyReviewers,
		},
		{
			desc:          "exclude-flakes with reviewers",
			workflow:      "exclude-flakes",
			reviewers:     dummyReviewers,
			wantReviewers: dummyReviewers,
		},
		{
			desc:          "rfd with reviewers",
			workflow:      "rfd",
			reviewers:     dummyReviewers,
			wantReviewers: dummyReviewers,
		},
		// local mode bypasses the reviewers requirement
		{
			desc:          "assign local no reviewers",
			workflow:      "assign",
			local:         true,
			wantReviewers: "",
		},
		{
			desc:          "check local no reviewers",
			workflow:      "check",
			local:         true,
			wantReviewers: "",
		},
		// workflows that do NOT need reviewers – omitting -reviewers → defaults to "{}"
		{
			desc:          "dismiss without reviewers",
			workflow:      "dismiss",
			wantReviewers: "{}",
		},
		{
			desc:          "label without reviewers",
			workflow:      "label",
			wantReviewers: "{}",
		},
		{
			desc:          "verify without reviewers",
			workflow:      "verify",
			wantReviewers: "{}",
		},
		{
			desc:          "binary-sizes without reviewers",
			workflow:      "binary-sizes",
			wantReviewers: "{}",
		},
		{
			desc:          "changelog without reviewers",
			workflow:      "changelog",
			wantReviewers: "{}",
		},
		{
			desc:          "docpaths without reviewers",
			workflow:      "docpaths",
			wantReviewers: "{}",
		},
		{
			desc:          "manual-test-plan without reviewers",
			workflow:      "manual-test-plan",
			wantReviewers: "{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

			args := []string{os.Args[0], "-token", dummyToken}
			if tt.workflow != "" {
				args = append(args, "-workflow", tt.workflow)
			}
			if tt.reviewers != "" {
				args = append(args, "-reviewers", tt.reviewers)
			}
			if tt.local {
				args = append(args, "-local")
			}
			os.Args = args

			got, err := parseFlags()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantReviewers, got.reviewers)
		})
	}
}
