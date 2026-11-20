/*
Copyright 2026 Gravitational, Inc.

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

package bot

import (
	"cmp"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/github"
)

// sizeHunkHeader matches the hunk header of a unified diff, capturing the starting line numbers of the old and new files.
var sizeHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

var licenseNotice = regexp.MustCompile(`(?i)\bcopyright\s+(?:\(c\)\s+|©\s+)?\d{4}\b|\blicensed under\b`)

// changesForSizeCheck excludes blank lines from both sides of a diff and license headers from newly added files.
// Subtract only exclusions visible in the patch, retaining GitHub's counts for changes omitted from a missing or
// truncated patch.
func changesForSizeCheck(file github.PullRequestFile) (additions, deletions int) {
	additions, deletions = file.Additions, file.Deletions

	var newHeader []string
	var inHunk, atHeader bool

	for _, line := range strings.Split(file.Patch, "\n") {
		if strings.HasPrefix(line, "@@") {
			// Parse the hunk header to determine if we are in a new file and at the start of the hunk.
			header := sizeHunkHeader.FindStringSubmatch(line)
			inHunk = header != nil
			atHeader = inHunk && file.Status == github.StatusAdded && header[2] == "1"

			continue
		}

		if !inHunk || len(line) == 0 {
			continue
		}

		text := strings.TrimSpace(line[1:])

		switch line[0] {
		case '-':
			if text == "" {
				deletions--
			}

		case '+':
			if text == "" {
				additions--
			}
			if atHeader {
				newHeader = append(newHeader, text)
			}
		}
	}

	additions = additions - licenseHeaderChanges(file.Name, newHeader)

	return max(0, additions), max(0, deletions)
}

// licenseHeaderChanges counts nonblank lines in a new file's leading license comments.
// Other leading comments, such as build tags and package docs, count toward the file's size.
func licenseHeaderChanges(name string, lines []string) int {
	var ignored int

	hashComments := usesHashComments(name, lines)

	for i := 0; i < len(lines); {
		text := lines[i]
		if text == "" || strings.HasPrefix(text, "#!") {
			// Skip blank lines and shebangs.
			i++

			continue
		}

		// Find the matching comment style for the license header.
		var end, prefix string
		var license bool
		switch {
		case strings.HasPrefix(text, "/*"):
			end = "*/"
		case strings.HasPrefix(text, "<!--"):
			end = "-->"
		case strings.HasPrefix(text, "//"):
			prefix = "//"
		case hashComments && strings.HasPrefix(text, "#"):
			prefix = "#"
		case strings.HasPrefix(text, "--"):
			prefix = "--"
		default:
			return ignored
		}

		var changed int
		var codeAfterComment bool

		for i < len(lines) {
			line := lines[i]
			if prefix != "" && !strings.HasPrefix(line, prefix) {
				break
			}

			i++

			// Handle Go build tags and directives specially, so we still count them as code changes.
			if prefix == "//" && (strings.HasPrefix(line, "//go:") ||
				strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(line, "//")), "+build ")) {

				continue
			}

			comment := line
			var closed bool

			if end != "" {
				var rest string

				comment, rest, closed = strings.Cut(comment, end)
				codeAfterComment = closed && strings.TrimSpace(rest) != ""
			}

			license = license || licenseNotice.MatchString(comment)
			if line != "" && !codeAfterComment {
				changed++
			}

			if closed {
				break
			}
		}

		if license {
			ignored += changed
		}

		if codeAfterComment {
			return ignored
		}
	}

	return ignored
}

// usesHashComments limits hash-style license headers to known file formats.
func usesHashComments(name string, lines []string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".sh", ".bash", ".zsh", ".py", ".pyi", ".rb", ".pl", ".pm",
		".yaml", ".yml", ".toml", ".tf", ".tfvars", ".hcl", ".mk", ".cmake", ".bzl", ".bazel", ".nix":
		return true
	}

	switch strings.ToLower(path.Base(name)) {
	case "makefile", "gnumakefile", "dockerfile", "cmakelists.txt":
		return true
	}

	return path.Ext(name) == "" && len(lines) > 0 && strings.HasPrefix(lines[0], "#!/")
}

const (
	// sizeReportDirDepth is the number of leading path elements used to group files in the size report.
	sizeReportDirDepth = 2
	// sizeReportMaxDirs caps the directory table so that a sweeping change does not flood the logs.
	sizeReportMaxDirs = 20
)

// sizeCounts holds the added and deleted lines of a group of files.
type sizeCounts struct {
	added, deleted int
}

func (c *sizeCounts) add(added, deleted int) {
	c.added += added
	c.deleted += deleted
}

func (c sizeCounts) net() int {
	return c.added - c.deleted
}

func (c sizeCounts) String() string {
	return fmt.Sprintf("+%d -%d", c.added, c.deleted)
}

// sizeGroup holds the line counts of a group of files, split by how the size check treats them.
type sizeGroup struct {
	files int
	// code and tests exclude blank lines and license headers. excluded holds GitHub's raw counts for files that
	// are skipped entirely.
	code, tests, excluded sizeCounts
}

// counted returns the number of lines the group contributes to the size of the PR.
func (g sizeGroup) counted() int {
	// Tests are cheaper to review than the code they cover, and counting them in full discourages writing them.
	// They still count for something so that a huge test-only change is not waved through.
	return g.code.net() + g.tests.net()/testFileSizeDivisor
}

// sizeReport explains how the size of a PR was calculated.
type sizeReport struct {
	label sizeLabel
	total sizeGroup
	// excludedFiles is the number of files that are skipped entirely.
	excludedFiles int
	// ignoredLines is the number of blank and license header lines left out of code and tests.
	ignoredLines int
	dirs         map[string]*sizeGroup
}

// measureSize calculates the size of a PR, keeping a breakdown of where the counted lines came from.
func measureSize(files []github.PullRequestFile) sizeReport {
	report := sizeReport{dirs: make(map[string]*sizeGroup)}

	for _, f := range files {
		name := sizeReportDir(f.Name)
		dir := report.dirs[name]
		if dir == nil {
			dir = &sizeGroup{}
			report.dirs[name] = dir
		}

		dir.files++
		report.total.files++

		if skipFileForSizeCheck(f.Name) {
			report.excludedFiles++
			dir.excluded.add(f.Additions, f.Deletions)
			report.total.excluded.add(f.Additions, f.Deletions)

			continue
		}

		added, deleted := changesForSizeCheck(f)
		report.ignoredLines += f.Additions + f.Deletions - added - deleted

		if isTestFileForSizeCheck(f.Name) {
			dir.tests.add(added, deleted)
			report.total.tests.add(added, deleted)
		} else {
			dir.code.add(added, deleted)
			report.total.code.add(added, deleted)
		}
	}

	report.label = sizeLabelFor(report.total.counted())

	return report
}

// sizeReportDir returns the directory a file is grouped under in the size report.
func sizeReportDir(name string) string {
	dir := path.Dir(name)
	if dir == "." {
		return "(root)"
	}

	parts := strings.Split(dir, "/")

	return strings.Join(parts[:min(len(parts), sizeReportDirDepth)], "/")
}

// String formats the report for the workflow logs, listing the directories that contribute the most first.
func (r sizeReport) String() string {
	var out strings.Builder

	fmt.Fprintf(&out, "PR size is %s: %d counted lines in %d files, admin approval is required at %d.\n",
		r.label, r.total.counted(), r.total.files, xlargeSizeThreshold)
	fmt.Fprintf(&out, "  code:     %v = %d\n", r.total.code, r.total.code.net())
	fmt.Fprintf(&out, "  tests:    %v = %d, counted at 1/%d = %d\n",
		r.total.tests, r.total.tests.net(), testFileSizeDivisor, r.total.tests.net()/testFileSizeDivisor)
	fmt.Fprintf(&out, "  excluded: %v in %d generated, vendored or fixture files\n", r.total.excluded, r.excludedFiles)
	fmt.Fprintf(&out, "  ignored:  %d blank and license header lines\n", r.ignoredLines)

	names := make([]string, 0, len(r.dirs))
	for name := range r.dirs {
		names = append(names, name)
	}
	// Deletions shrink the PR size, so rank by magnitude to keep directories with large removals visible.
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(
			cmp.Compare(sizeReportWeight(*r.dirs[b]), sizeReportWeight(*r.dirs[a])),
			cmp.Compare(a, b),
		)
	})

	rows := [][]string{{"DIRECTORY", "FILES", "CODE", "TESTS", "EXCLUDED", "COUNTED"}}

	var rest sizeGroup
	for i, name := range names {
		dir := *r.dirs[name]
		if i >= sizeReportMaxDirs {
			rest.files += dir.files
			rest.code.add(dir.code.added, dir.code.deleted)
			rest.tests.add(dir.tests.added, dir.tests.deleted)
			rest.excluded.add(dir.excluded.added, dir.excluded.deleted)

			continue
		}

		rows = append(rows, sizeReportRow(name, dir))
	}
	if len(names) > sizeReportMaxDirs {
		rows = append(rows, sizeReportRow(fmt.Sprintf("(%d more)", len(names)-sizeReportMaxDirs), rest))
	}

	// Halving tests per directory rounds differently than halving them once, so the total row is not always the sum
	// of the rows above it.
	rows = append(rows, sizeReportRow("TOTAL", r.total))

	writeSizeReportTable(&out, rows)

	return strings.TrimRight(out.String(), "\n")
}

func sizeReportRow(name string, g sizeGroup) []string {
	return []string{
		name,
		strconv.Itoa(g.files),
		g.code.String(),
		g.tests.String(),
		g.excluded.String(),
		strconv.Itoa(g.counted()),
	}
}

// writeSizeReportTable draws rows as a bordered table, treating the first row as the header and the last as the footer.
// The first column is left aligned and the rest are right aligned.
func writeSizeReportTable(out *strings.Builder, rows [][]string) {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], len(cell))
		}
	}

	border := "  +"
	for _, width := range widths {
		border += strings.Repeat("-", width+2) + "+"
	}

	for i, row := range rows {
		if i <= 1 || i == len(rows)-1 {
			fmt.Fprintln(out, border)
		}

		out.WriteString("  |")
		for j, cell := range row {
			if j == 0 {
				fmt.Fprintf(out, " %-*s |", widths[j], cell)
			} else {
				fmt.Fprintf(out, " %*s |", widths[j], cell)
			}
		}
		out.WriteString("\n")
	}

	fmt.Fprintln(out, border)
}

func sizeReportWeight(g sizeGroup) int {
	weight := g.counted()
	if weight < 0 {
		return -weight
	}

	return weight
}
