package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)


func TestAddSummary(t *testing.T) {
	resetSummaryState()

	AddSummary("build", SummaryParagraph{Msg: "first build entry"})
	AddSummary("test", SummaryParagraph{Msg: "test entry"})
	AddSummary("build", SummaryParagraph{Msg: "second build entry"})

	require.Equal(t, []string{"build", "test"}, summary.steps)
	require.Len(t, summary.stepStatuses["build"], 2)
	require.Len(t, summary.stepStatuses["test"], 1)
}

func TestPrintSummaryReport(t *testing.T) {
	resetSummaryState()

	summaryFile := filepath.Join(t.TempDir(), "step_summary.html")
	t.Setenv(GitHubStepSummary, summaryFile)

	AddSummary("build", SummaryRow{Result: SummaryResultSuccess, Msg: "build passed"})
	AddSummary("deploy", SummaryParagraph{Msg: "deployment skipped"})

	PrintSummaryReport("Workflow Summary")

	contents, err := os.ReadFile(summaryFile)
	require.NoError(t, err)
	require.Equal(t, strings.Join([]string{
		"<details>",
		"<summary><h2>Workflow Summary</h2></summary>",
		"<p><h3>build</h3></p>",
		"<table><tr><th>Result</th><th>Message</th></tr>",
		"<tr><td>✅</td><td>build passed</td></tr>",
		"</table>",
		"<p><h3>deploy</h3></p>",
		"<p>deployment skipped</p>",
		"",
		"</details>",
		"",
	}, "\n"), string(contents))
}

func resetSummaryState() {
	summary = summaryReporter{
		steps:        []string{},
		stepStatuses: make(map[string][]SummaryReportable),
	}
}
