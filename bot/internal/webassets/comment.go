package webassets

import (
	"fmt"
	"io"
	"math"
	"strings"
)

const (
	commentHeader = "## 📦 Bundle Size Report"
)

const (
	subHeadingLevel = 3
)

func IsBotComment(comment string) bool {
	return strings.HasPrefix(comment, commentHeader)
}

func renderHeader(w io.Writer) {
	fmt.Fprintf(w, "%s\n\n", commentHeader)
}

func renderHeading(w io.Writer, level int, text string) {
	if level < 1 || level > 6 {
		level = 1
	}
	fmt.Fprintf(w, "%s %s\n", strings.Repeat("#", level), text)
}

func renderNewBundles(w io.Writer, bundles []bundleChange) {
	fmt.Fprint(w, "### 🆕 New Bundles\n")

	headers := []string{"Bundle", "Size"}
	rows := make([][]string, len(bundles))

	for i, bc := range bundles {
		rows[i] = []string{
			fmt.Sprintf("`%s`", bc.name),
			fmt.Sprintf("%s (%s gz)", formatBytes(bc.size), formatBytes(bc.gzipSize)),
		}
	}

	fmt.Fprint(w, generateMarkdownTable(headers, rows))
	fmt.Fprint(w, "\n\n")
}

func renderBundleChanges(w io.Writer, changes []bundleChange) {
	fmt.Fprint(w, "### 🎯 Bundle Changes\n")

	headers := []string{":Bundle", "Size:", "Δ Size:", "Gzipped:", "Δ Gzipped:"}
	rows := make([][]string, len(changes))

	for i, bc := range changes {
		rows[i] = []string{
			bold(code(bc.name)),
			code(formatBytes(bc.size)),
			formatChangeColumn(bc.change, true),
			code(formatBytes(bc.gzipSize)),
			formatChangeColumn(bc.gzipChange, true),
		}
	}

	fmt.Fprint(w, generateMarkdownTable(headers, rows))
	fmt.Fprint(w, "\n\n")
}

func renderNewDependencies(w io.Writer, modules []moduleChange) {
	fmt.Fprint(w, "### 🆕 New Dependencies\n")

	limit := newDependenciesLimit
	if len(modules) < limit {
		limit = len(modules)
	}

	headers := []string{":Package", "Size:", "Gzipped:", ":Impact:"}
	rows := make([][]string, 0, limit+1)

	for i := 0; i < limit; i++ {
		module := modules[i]
		rows = append(rows, []string{
			bold(code(module.name)),
			code(formatBytes(module.size)),
			code(formatBytes(module.gzipSize)),
			getChangeColor(module.gzipSize, "module", true),
		})
	}

	if len(modules) > limit {
		rows = append(rows, []string{
			fmt.Sprintf("_...and %d more_", len(modules)-limit),
			"",
		})
	}

	fmt.Fprint(w, generateMarkdownTable(headers, rows))
	fmt.Fprint(w, "\n\n")
}

func renderIncreasedDependencies(w io.Writer, modules []moduleChange) {
	fmt.Fprint(w, "### 📈 Increased Dependencies (>5KB)\n")

	limit := increasedDependenciesLimit
	if len(modules) < limit {
		limit = len(modules)
	}

	headers := []string{":Package", "Size:", "Δ Size:", "Gzipped:", "Δ Gzipped:"}
	rows := make([][]string, 0, limit+1)

	for i := 0; i < limit; i++ {
		module := modules[i]
		rows = append(rows, []string{
			bold(code(module.name)),
			code(formatBytes(module.size)),
			formatChangeColumn(module.change, false),
			code(formatBytes(module.gzipSize)),
			formatChangeColumn(module.gzipChange, true),
		})
	}

	if len(modules) > limit {
		rows = append(rows, []string{
			fmt.Sprintf("_...and %d more_", len(modules)-limit),
			"",
			"",
		})
	}

	fmt.Fprint(w, generateMarkdownTable(headers, rows))
	fmt.Fprint(w, "\n\n")
}

func renderNewFiles(w io.Writer, files []moduleChange) {
	limit := newFilesLimit
	if len(files) < limit {
		limit = len(files)
	}

	fmt.Fprint(w, "### 📄 New Files\n")

	headers := []string{":File", "Size:", "Gzipped:"}
	rows := make([][]string, 0, limit+1)

	for i := 0; i < limit; i++ {
		file := files[i]
		rows = append(rows, []string{
			code(file.name),
			code(formatBytes(file.size)),
			code(formatBytes(file.gzipSize)),
		})
	}

	if len(files) > limit {
		rows = append(rows, []string{
			fmt.Sprintf("_...and %d more_", len(files)-limit),
			"",
		})
	}

	fmt.Fprint(w, generateMarkdownTable(headers, rows))
	fmt.Fprint(w, "\n\n")
}

func renderDetailsStart(w io.Writer, title string) {
	fmt.Fprintf(w, "<details>\n")
	fmt.Fprintf(w, "<summary><strong>%s</strong></summary>\n\n", title)
}

func renderDetailsEnd(w io.Writer) {
	fmt.Fprint(w, "\n\n</details>\n")
}

func renderTopDependencies(w io.Writer, modules []moduleChange) {
	fmt.Fprintf(w, "### 🏆 Largest %d Dependencies\n", numberOfTopModules)

	limit := numberOfTopModules
	if len(modules) < limit {
		limit = len(modules)
	}

	headers := []string{" :", ":Package", "Size:", "Gzipped:"}
	rows := make([][]string, limit)

	for i := 0; i < limit; i++ {
		module := modules[i]
		rows[i] = []string{
			fmt.Sprintf("%d", i+1),
			bold(code(module.name)),
			code(formatBytes(module.size)),
			code(formatBytes(module.gzipSize)),
		}
	}

	fmt.Fprint(w, generateMarkdownTable(headers, rows))
}

func renderSubHeading(w io.Writer, text string) {
	renderHeading(w, subHeadingLevel, text)
}

func numberSign(number int64) string {
	if number < 0 {
		return "-"
	}
	return "+"
}

func renderDivider(w io.Writer) {
	fmt.Fprint(w, "---\n\n")
}

func formatChangeColumn(change sizeChange, gzipped bool) string {
	sign := numberSign(change.diff)

	return fmt.Sprintf("`%s%s (%s%s%%)` %s",
		sign,
		formatBytes(change.diff),
		sign,
		fmt.Sprintf("%.1f", change.percentChange),
		getChangeColor(change.diff, "file", gzipped))
}

func bold(text string) string {
	return fmt.Sprintf("**%s**", text)
}

func code(text string) string {
	return fmt.Sprintf("`%s`", text)
}

func renderSummary(w io.Writer, totalChange, totalGzipChange sizeChange) {
	headers := []string{"", "Size:", "Change:"}
	rows := [][]string{
		{
			"**Total Size**",
			code(formatBytes(totalChange.after)),
			formatChangeColumn(totalChange, false),
		},
		{
			"**Gzipped**",
			code(formatBytes(totalGzipChange.after)),
			formatChangeColumn(totalGzipChange, true),
		},
	}

	fmt.Fprint(w, generateMarkdownTable(headers, rows))
	fmt.Fprint(w, "\n\n")
}

func formatBytes(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	sizes := []string{"B", "KB", "MB", "GB", "TB"}
	i := int(math.Floor(math.Log(float64(bytes)) / math.Log(1024)))

	if i >= len(sizes) {
		i = len(sizes) - 1
	}

	return fmt.Sprintf("%.2f %s", float64(bytes)/math.Pow(1024, float64(i)), sizes[i])
}

func getChangeColor(diff int64, thresholdType string, gzipped bool) string {
	if diff < 0 {
		return "🟢"
	}

	var warningThreshold, dangerThreshold int64
	switch thresholdType {
	case "file":
		if gzipped {
			warningThreshold = fileGzipThresholdWarning
			dangerThreshold = fileGzipThresholdDanger
		} else {
			warningThreshold = fileThresholdWarning
			dangerThreshold = fileThresholdDanger
		}
	default:
		warningThreshold = moduleThresholdWarning
		dangerThreshold = moduleThresholdDanger
	}

	if diff > dangerThreshold {
		return "🔴"
	}

	if diff > warningThreshold {
		return "🟡"
	}

	return "🟢"
}

func getChangeIndicator(change sizeChange, thresholdType string, gzipped bool) string {
	if change.diff == 0 {
		return "➡️"
	}

	if change.diff > significantIncreaseThreshold {
		return fmt.Sprintf("⬆️ %s", getChangeColor(change.diff, thresholdType, gzipped))
	}

	if change.diff < 0 {
		return "⬇️ 🟢"
	}

	return "⬆️ 🟢"
}
