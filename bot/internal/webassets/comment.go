package webassets

import (
	"fmt"
	"io"
	"math"
	"strings"
)

const (
	commentHeader = "# 📦 Bundle Size Report"
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
	fmt.Fprint(w, "### 📊 Bundle Changes\n")

	headers := []string{"Bundle", "Size", "Change"}
	rows := make([][]string, len(changes))

	for i, bc := range changes {
		rows[i] = []string{
			fmt.Sprintf("`%s`", bc.name),
			fmt.Sprintf("%s (%s gz)", formatBytes(bc.size), formatBytes(bc.gzipSize)),
			formatSizeChange(bc.change, "file", false),
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

	headers := []string{"Package", "Size"}
	rows := make([][]string, 0, limit+1)

	for i := 0; i < limit; i++ {
		module := modules[i]
		rows = append(rows, []string{
			fmt.Sprintf("`%s`", module.name),
			formatSize(module.size, module.gzipSize),
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

	headers := []string{"Package", "Size", "Change"}
	rows := make([][]string, 0, limit+1)

	for i := 0; i < limit; i++ {
		module := modules[i]
		color := getChangeColor(module.change.diff, "module", false)
		rows = append(rows, []string{
			fmt.Sprintf("`%s`", module.name),
			formatSize(module.size, module.gzipSize),
			fmt.Sprintf("+%s (+%.1f%%) ⬆️ %s",
				formatBytes(module.change.diff),
				module.change.percentChange,
				color),
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

	fmt.Fprintf(w, "### 📄 New Files (Top %d)\n", limit)

	headers := []string{"File", "Size"}
	rows := make([][]string, 0, limit+1)

	for i := 0; i < limit; i++ {
		file := files[i]
		rows = append(rows, []string{
			fmt.Sprintf("`%s`", file.name),
			formatSize(file.size, file.gzipSize),
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
	fmt.Fprintf(w, "### Top %d Dependencies\n", numberOfTopModules)

	limit := numberOfTopModules
	if len(modules) < limit {
		limit = len(modules)
	}

	headers := []string{"Package", "Size"}
	rows := make([][]string, limit)

	for i := 0; i < limit; i++ {
		module := modules[i]
		rows[i] = []string{
			fmt.Sprintf("`%s`", module.name),
			formatSize(module.size, module.gzipSize),
		}
	}

	fmt.Fprint(w, generateMarkdownTable(headers, rows))
}

func renderSubHeading(w io.Writer, text string) {
	renderHeading(w, subHeadingLevel, text)
}

func renderSummary(w io.Writer, totalChange, totalGzipChange sizeChange) {
	renderSubHeading(w, "Summary")

	fmt.Fprintf(w, "**Total Size:** %s\n", formatSizeChange(totalChange, "file", false))
	fmt.Fprintf(w, "**Gzipped:** %s\n\n", formatSizeChange(totalGzipChange, "file", true))
}

func calculateCompressionRatio(totalSize, totalGzipSize int64) float64 {
	if totalSize == 0 {
		return 0.0
	}
	return float64(totalSize-totalGzipSize) / float64(totalSize) * 100
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

func formatSize(size, gzipSize int64) string {
	return fmt.Sprintf("%s (%s gzipped)", formatBytes(size), formatBytes(gzipSize))
}

func formatSizeChange(change sizeChange, thresholdType string, gzipped bool) string {
	sign := "-"
	percentSign := ""
	if change.diff >= 0 {
		sign = "+"
		percentSign = "+"
	}

	percentStr := fmt.Sprintf("%.1f", change.percentChange)
	indicator := getChangeIndicator(change, thresholdType, gzipped)

	abs := int64(math.Abs(float64(change.diff)))

	if change.isNew {
		return fmt.Sprintf("🆕 %s (%s%s)",
			formatBytes(change.after),
			sign,
			formatBytes(abs))
	}

	return fmt.Sprintf("%s (%s%s, %s%s%%) %s",
		formatBytes(change.after),
		sign,
		formatBytes(abs),
		percentSign,
		percentStr,
		indicator)
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
