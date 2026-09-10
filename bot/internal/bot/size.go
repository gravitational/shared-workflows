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
	"path"
	"regexp"
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
