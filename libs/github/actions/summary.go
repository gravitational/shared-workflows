package actions

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const (
	SummaryResultSuccess = iota
	SummaryResultFailure
	SummaryResultWarning
	SummaryResultInfo
)

const (
	GitHubStepSummary = "GITHUB_STEP_SUMMARY"
	GitHubEnv         = "GITHUB_ENV"
)

// SummaryReportable is an interface that can be implemented to define how a summary entry 
// should be displayed in the GitHub Actions summary report. The header and footer will be
// taken from the first entry for each step, and the row will be printed for each entry.
type SummaryReportable interface {
	Header() string
	Row() string
	Footer() string
}

type SummaryRow struct {
	Result int
	Msg    string
}

func (r SummaryRow) Header() string {
	return "<table><tr><th>Result</th><th>Message</th></tr>\n"
}

func (r SummaryRow) Row() string {
	return fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>\n", emojiForResult(r.Result), r.Msg)
}

func (r SummaryRow) Footer() string {
	return "</table>\n"
}

type SummaryRowWithCounts struct {
	Result       int
	Msg          string
	SuccessCount int
	WarningCount int
	FailureCount int
}

func (r SummaryRowWithCounts) Header() string {
	return "<table><tr><th>Result</th><th>Message</th><th>✅</th><th>⚠️</th><th>❌</th></tr>\n"
}

func (r SummaryRowWithCounts) Row() string {	
	return fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td></tr>\n", emojiForResult(r.Result), r.Msg, r.SuccessCount, r.WarningCount, r.FailureCount)
}

func (r SummaryRowWithCounts) Footer() string{	
	return "</table>\n"
}

type SummaryParagraph struct {
	Msg string
}

func (p SummaryParagraph) Header() string {
	return ""
}

func (p SummaryParagraph) Row() string {
	return fmt.Sprintf("<p>%s</p>\n", p.Msg)
}

func (p SummaryParagraph) Footer() string {
	return ""
}

func emojiForResult(result int) string {
	switch result {
	case SummaryResultFailure:
		return "❌"
	case SummaryResultWarning:
		return "⚠️"
	case SummaryResultInfo:
		return "ℹ️"
	case SummaryResultSuccess:
		return "✅"
	}
	return ""
}

type summaryReporter struct {
	steps        []string
	stepStatuses map[string][]SummaryReportable
}

var (
	summary = summaryReporter{
		steps:        []string{},
		stepStatuses: make(map[string][]SummaryReportable),
	}
)

// PrintSummaryReport writes the summary report to the file specified by the GITHUB_STEP_SUMMARY
// environment variable, which will be displayed in the GitHub Actions UI. The report will include
// all entries added via AddSummary, grouped by step name. The title parameter will be displayed
// at the top of the summary. Each step's header and footer will be taken from the first entry
// added for that step.
func PrintSummaryReport(title string) {
	summary.reportSummary(title)
}

// reportSummary prints a summary of the steps and their results to the file identified by the
// GITHUB_STEP_SUMMARY environment variable, which will be displayed in the GitHub Actions UI.
func (r *summaryReporter) reportSummary(title string) {
	output := strings.Builder{}
	output.WriteString(fmt.Sprintf("<details>\n<summary><h2>%s</h2></summary>\n", title))

	for _, step := range r.steps {
		statuses := r.stepStatuses[step]
		if len(statuses) == 0 {
			continue
		}
		output.WriteString(fmt.Sprintf("<p><h3>%s</h3></p>\n", step))
		if len(statuses) == 0 {
			continue
		}

		output.WriteString(statuses[0].Header())
		for _, status := range statuses {
			output.WriteString(status.Row())
		}
		output.WriteString(statuses[0].Footer())
	}
	output.WriteString("\n</details>\n")

	summaryFile := os.Getenv(GitHubStepSummary)
	if summaryFile == "" {
		slog.Error("step summary environment variable not set, cannot write summary report", "variable", GitHubStepSummary)
		return
	}

	if err := os.WriteFile(summaryFile, []byte(output.String()), 0644); err != nil {
		slog.Error("Error writing GitHub summary file", "error", err, "file", summaryFile)
	}
}

// AddSummary adds an entry to the GitHub Actions summary report.
func AddSummary(stepName string, status SummaryReportable) {
	if _, ok := summary.stepStatuses[stepName]; !ok {
		summary.steps = append(summary.steps, stepName)
	}
	if _, ok := summary.stepStatuses[stepName]; !ok {
		summary.stepStatuses[stepName] = []SummaryReportable{}
	}
	summary.stepStatuses[stepName] = append(summary.stepStatuses[stepName], status)
}
