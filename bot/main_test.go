package main

import (
	"encoding/base64"
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlagsReviewers(t *testing.T) {
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

	assert.Equal(t, b64Flags.reviewers, plainTextFlags.reviewers)
}
