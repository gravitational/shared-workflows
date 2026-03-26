package github

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
)

const (
	StepResultSuccess = iota
	StepResultFailure
	StepResultWarning
)

// StepStatus is used to track the result of each step for summary reporting at the end of execution.
type StepStatus struct {
	Result       int
	Msg          string
	SuccessCount int
	FailureCount int
	WarningCount int
}

type summaryReporter struct {
	steps        []string
	stepStatuses map[string][]StepStatus
}

// GhaEnvValue combines the configuration for an environment variable with its corresponding value retrieved from Secrets Manager
// along with helper methods for generating GitHub Actions outputs.
type GhaEnvValue struct {
	VarName   string
	ValueType string
	Value     string
}

// MaskValue returns the value to be masked, or an empty string if this value should not be masked.
func (e GhaEnvValue) MaskValue() string {
	if e.ValueType == "secret" && e.Value != "" {
		return e.Value
	}
	return ""
}

// EnvVarAssignment returns the string to be written to GITHUB_ENV to set this environment
// variable for subsequent steps in the GitHub Actions workflow.
//
// from: https://github.com/actions/toolkit/blob/44d43b5490b02998bd09b0c4ff369a4cc67876c2/packages/core/src/file-command.ts#L27-L47
func (e GhaEnvValue) EnvVarAssignment() string {
	delimiter := fmt.Sprintf("ghadelimiter_%s", uuid.New().String())
	// panic if there's a collision with the random delimeter since it's an unrecoverable state
	if strings.Contains(e.VarName, delimiter) {
		panic(fmt.Sprintf("Unexpected input: VarName should not contain the delimiter '%s'", delimiter))
	}
	if strings.Contains(e.Value, delimiter) {
		panic(fmt.Sprintf("Unexpected input: Value should not contain the delimiter '%s'", delimiter))
	}
	return fmt.Sprintf("%s<<%s\n%s\n%s", e.VarName, delimiter, e.Value, delimiter)
}

var (
	summary = summaryReporter{}
)

func init() {
	summary = summaryReporter{
		steps:        []string{},
		stepStatuses: make(map[string][]StepStatus),
	}
}

func PrintSummaryReport() {
	summary.ReportSummary()
}

// ReportSummary prints a summary of the steps and their results to the file identified by the
// GITHUB_STEP_SUMMARY environment variable, which will be displayed in the GitHub Actions UI.
func (r *summaryReporter) ReportSummary() {
	output := strings.Builder{}
	output.WriteString("# env-kvstore - Environment Variable Retrieval Summary\n\n")
	for _, step := range r.steps {
		statuses := r.stepStatuses[step]
		if len(statuses) == 0 {
			continue
		}
		output.WriteString("## " + step + "\n")
		// output a table header with columns for stepName, result, msg, successCount, failureCount, warningCount
		output.WriteString("| Result | Message | ✅ | ❌ | ⚠️ |\n")
		output.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, status := range statuses {
			resultEmoji := "✅"
			switch status.Result {
			case StepResultFailure:
				resultEmoji = "❌"
			case StepResultWarning:
				resultEmoji = "⚠️"
			}
			output.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d |\n", resultEmoji, status.Msg, status.SuccessCount, status.FailureCount, status.WarningCount))
		}
	}
	summaryFile := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryFile == "" {
		slog.Error("GITHUB_STEP_SUMMARY environment variable not set, cannot write summary report")
		return
	}

	f, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("Error opening GITHUB_STEP_SUMMARY file", "error", err)
		return
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(output.String()); err != nil {
		slog.Error("Error writing to GITHUB_STEP_SUMMARY file", "error", err)
	}
}

// AddSummary adds the status of a step to the GitHub Actions summary report.
func AddSummary(stepName string, status StepStatus) {
	if _, ok := summary.stepStatuses[stepName]; !ok {
		summary.steps = append(summary.steps, stepName)
	}
	if _, ok := summary.stepStatuses[stepName]; !ok {
		summary.stepStatuses[stepName] = []StepStatus{}
	}
	summary.stepStatuses[stepName] = append(summary.stepStatuses[stepName], status)
}

// AppendEnvLines appends environment variable definitions to the file specified by the GITHUB_ENV
// environment variable, which will set those environment variables for subsequent steps in the
// GitHub Actions workflow.
func AppendEnvLines(values *[]GhaEnvValue) error {
	envFile := os.Getenv("GITHUB_ENV")
	if envFile == "" {
		slog.Error("GITHUB_ENV environment variable not set, cannot append environment variables")
		return fmt.Errorf("GITHUB_ENV environment variable not set")
	}
	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("Error opening GITHUB_ENV file", "error", err)
		return fmt.Errorf("error opening GITHUB_ENV file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(GenerateEnvLines(values)); err != nil {
		slog.Error("Error writing to GITHUB_ENV file", "error", err)
		return fmt.Errorf("error writing to GITHUB_ENV file: %w", err)
	}
	return nil
}

func GenerateEnvLines(values *[]GhaEnvValue) string {
	var sb strings.Builder
	for _, v := range *values {
		sb.WriteString(v.EnvVarAssignment())
		sb.WriteString("\n")
	}
	return sb.String()
}

func MaskSecretValues(values *[]GhaEnvValue) {
	fmt.Println()
	for _, m := range *values {
		if v := m.MaskValue(); v != "" {
			fmt.Printf("::add-mask::%s\n", v)
		}
	}
}
