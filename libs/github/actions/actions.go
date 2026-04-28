// actions provides utility functions for GitHub Actions workflows
package actions

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
)

const (
	GithubEnv         = "GITHUB_ENV"
	GithubOutput	  = "GITHUB_OUTPUT"
	GithubState	  	  = "GITHUB_STATE"
)

// MaskSecretValues takes in a slice of secret values and prints GitHub Actions commands to mask
// those values from log outputs.
func MaskSecretValues(secrets []string) {
	fmt.Println()
	for _, v := range secrets {
		if v == "" {
			continue
		}
		fmt.Printf("::add-mask::%s\n", v)
		stdEncoded := base64.StdEncoding.EncodeToString([]byte(v))
		fmt.Printf("::add-mask::%s\n", stdEncoded)
		urlEncoded := base64.URLEncoding.EncodeToString([]byte(v))
		fmt.Printf("::add-mask::%s\n", urlEncoded)
	}
}

// WriteGithubEnv writes environment variables to the file specified by the GITHUB_ENV
// environment variable for use in subsequent steps in the GitHub Actions workflow.
// Note: this will overwrite the file if it already exists and does not append to it.
func WriteGithubEnv(variables map[string]string) error {
	envFile := os.Getenv(GithubEnv)
	if envFile == "" {
		return fmt.Errorf("%s environment variable not set", GithubEnv)
	}
	if err := writeKVFile(envFile, variables); err != nil {
		return fmt.Errorf("error writing environment variables to file: %w", err)
	}

	return nil
}

// WriteGithubOutput writes output variables to the file specified by the GITHUB_OUTPUT
// environment variable for use in subsequent steps and jobs in the GitHub Actions workflow.
// Note: this will overwrite the file if it already exists and does not append to it.
func WriteGithubOutput(outputs map[string]string) error {
	outputFile := os.Getenv(GithubOutput)
	if outputFile == "" {
		return fmt.Errorf("%s environment variable not set", GithubOutput)
	}
	if err := writeKVFile(outputFile, outputs); err != nil {
		return fmt.Errorf("error writing output variables to file: %w", err)
	}

	return nil
}

// WriteGithubState writes state variables to the file specified by the GITHUB_STATE
// environment variable for use in cleanup steps of a GitHub Actions workflow.
// Note: this will overwrite the file if it already exists and does not append to it.
func WriteGithubState(states map[string]string) error {
	stateFile := os.Getenv(GithubState)
	if stateFile == "" {
		return fmt.Errorf("%s environment variable not set", GithubState)
	}
	if err := writeKVFile(stateFile, states); err != nil {
		return fmt.Errorf("error writing state variables to file: %w", err)
	}

	return nil
}

func writeKVFile(filePath string, kv map[string]string) error {
	output, err := generateKVContents(kv)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filePath, []byte(output), 0644); err != nil {
		return fmt.Errorf("error writing to file %s: %w", filePath, err)
	}
	return nil
}

// generateKVContents returns the contents to be written to any of the output files
// where variable are set for subsequent steps in the GitHub Actions workflow.
func generateKVContents(values map[string]string) (string, error) {
	var sb strings.Builder
	for k, v := range values {
		line, err := generateKVAssignment(k, v)
		if err != nil {
			return "", err			
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func generateKVAssignment(name, value string) (string, error) {
	// adapted from https://github.com/actions/toolkit/blob/44d43b5490b02998bd09b0c4ff369a4cc67876c2/packages/core/src/file-command.ts#L27-L47
	delimiter := fmt.Sprintf("ghadelimiter_%s", uuid.New().String())
	if strings.Contains(name, delimiter) {
		return "", fmt.Errorf("unexpected input: VarName should not contain the delimiter '%s'", delimiter)
	}
	if strings.Contains(value, delimiter) {
		return "", fmt.Errorf("unexpected input: Value should not contain the delimiter '%s'", delimiter)
	}
	return fmt.Sprintf("%s<<%s\n%s\n%s", name, delimiter, value, delimiter), nil
}
