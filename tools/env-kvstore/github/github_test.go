package github

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGhaEnvValueMaskValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    GhaEnvValue
		expected string
	}{
		{
			name: "secret with value",
			value: GhaEnvValue{
				VarName:   "SECRET_TOKEN",
				ValueType: "secret",
				Value:     "super-secret",
			},
			expected: "super-secret",
		},
		{
			name: "secret with empty value",
			value: GhaEnvValue{
				VarName:   "EMPTY_SECRET",
				ValueType: "secret",
			},
			expected: "",
		},
		{
			name: "variable is not masked",
			value: GhaEnvValue{
				VarName:   "PUBLIC_VALUE",
				ValueType: "variable",
				Value:     "visible",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.value.MaskValue(); got != tt.expected {
				t.Fatalf("MaskValue() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGhaEnvValueEnvVarAssignment(t *testing.T) {
	t.Parallel()

	value := GhaEnvValue{
		VarName:   "MULTILINE_SECRET",
		ValueType: "secret",
		Value:     "line1\nline2",
	}

	got := value.EnvVarAssignment()
	pattern := regexp.MustCompile(`^MULTILINE_SECRET<<ghadelimiter_[0-9a-f-]+\nline1\nline2\nghadelimiter_[0-9a-f-]+$`)
	if !pattern.MatchString(got) {
		t.Fatalf("EnvVarAssignment() returned unexpected format:\n%s", got)
	}
}

func TestGenerateEnvLines(t *testing.T) {
	t.Parallel()

	values := []GhaEnvValue{
		{
			VarName:   "FIRST",
			ValueType: "variable",
			Value:     "alpha",
		},
		{
			VarName:   "SECOND",
			ValueType: "secret",
			Value:     "beta\ngamma",
		},
	}

	got := GenerateEnvLines(&values)
	if !strings.Contains(got, "FIRST<<ghadelimiter_") {
		t.Fatalf("GenerateEnvLines() missing FIRST assignment:\n%s", got)
	}
	if !strings.Contains(got, "alpha\n") {
		t.Fatalf("GenerateEnvLines() missing FIRST value:\n%s", got)
	}
	if !strings.Contains(got, "SECOND<<ghadelimiter_") {
		t.Fatalf("GenerateEnvLines() missing SECOND assignment:\n%s", got)
	}
	if !strings.Contains(got, "beta\ngamma\n") {
		t.Fatalf("GenerateEnvLines() missing SECOND value:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("GenerateEnvLines() should end with newline, got %q", got)
	}
}

func TestAppendEnvLines(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "github_env")
	if err := os.WriteFile(envFile, []byte("EXISTING=1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("GITHUB_ENV", envFile)

	values := []GhaEnvValue{
		{
			VarName:   "APPENDED",
			ValueType: "variable",
			Value:     "value",
		},
	}

	if err := AppendEnvLines(&values); err != nil {
		t.Fatalf("AppendEnvLines() error = %v", err)
	}

	contents, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(contents)
	if !strings.HasPrefix(got, "EXISTING=1\n") {
		t.Fatalf("AppendEnvLines() overwrote existing file contents:\n%s", got)
	}
	if !strings.Contains(got, "APPENDED<<ghadelimiter_") {
		t.Fatalf("AppendEnvLines() missing appended variable:\n%s", got)
	}
}

func TestReportSummaryAppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	summaryFile := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(summaryFile, []byte("existing summary\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("GITHUB_STEP_SUMMARY", summaryFile)

	oldSummary := summary
	summary = summaryReporter{
		steps:        []string{},
		stepStatuses: make(map[string][]StepStatus),
	}
	t.Cleanup(func() {
		summary = oldSummary
	})

	AddSummary("Retrieve values", StepStatus{
		Result:       StepResultSuccess,
		Msg:          "loaded values",
		SuccessCount: 2,
	})

	PrintSummaryReport()

	contents, err := os.ReadFile(summaryFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(contents)
	if !strings.HasPrefix(got, "existing summary\n") {
		t.Fatalf("PrintSummaryReport() should append to existing summary:\n%s", got)
	}
	if !strings.Contains(got, "# env-kvstore - Environment Variable Retrieval Summary") {
		t.Fatalf("PrintSummaryReport() missing summary header:\n%s", got)
	}
	if !strings.Contains(got, "## Retrieve values") {
		t.Fatalf("PrintSummaryReport() missing step section:\n%s", got)
	}
	if !strings.Contains(got, "| ✅ | loaded values | 2 | 0 | 0 |") {
		t.Fatalf("PrintSummaryReport() missing status row:\n%s", got)
	}
}

