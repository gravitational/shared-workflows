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
	"fmt"
	"strings"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/stretchr/testify/require"
)

const apacheSizeHeader = `/*
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
`

const agplSizeHeader = `/*
 * Teleport
 * Copyright (C) 2026 Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */
`

func TestChangesForSizeCheck(t *testing.T) {
	for _, test := range []struct {
		name      string
		filename  string
		file      github.PullRequestFile
		additions int
		deletions int
	}{
		{
			name:      "Apache header in a new file",
			file:      sizeTestFile("", apacheSizeHeader+"\npackage example\n"),
			additions: 1,
		},
		{
			name:      "AGPL header with star prefixes",
			file:      sizeTestFile("", agplSizeHeader+"\npackage example\n"),
			additions: 1,
		},
		{
			name: "license changes in existing files still count",
			file: sizeTestFile("/* Copyright 2025 Gravitational, Inc. */\n\nold()\n",
				"/* Copyright 2026 Gravitational, Inc. */\n\nnew()\n"),
			additions: 2,
			deletions: 2,
		},
		{
			name:     "line comments after a shebang",
			filename: "script.sh",
			file: sizeTestFile("", "#!/bin/sh\n\n# Copyright 2026 Gravitational, Inc.\n"+
				"#\n# Licensed under the Apache License, Version 2.0\n\necho hello\n"),
			additions: 2,
		},
		{
			name:      "hash license in a Python file",
			filename:  "script.py",
			file:      sizeTestFile("", "# Copyright 2026 Gravitational, Inc.\n# All rights reserved.\n\nprint('hello')\n"),
			additions: 1,
		},
		{
			name:      "hash license in a Makefile",
			filename:  "build/Makefile",
			file:      sizeTestFile("", "# Copyright 2026 Gravitational, Inc.\n# All rights reserved.\n\nVERSION = 1\n"),
			additions: 1,
		},
		{
			name:      "hash license in an extensionless script",
			filename:  "scripts/build",
			file:      sizeTestFile("", "#!/bin/sh\n# Copyright 2026 Gravitational, Inc.\n# All rights reserved.\n\necho hello\n"),
			additions: 2,
		},
		{
			name: "copyright line comments",
			file: sizeTestFile("", "// Copyright 2026 Gravitational, Inc.\n"+
				"// See LICENSE for details.\n\npackage example\n"),
			additions: 1,
		},
		{
			name:      "SQL license comments",
			file:      sizeTestFile("", "-- Copyright 2026 Gravitational, Inc.\n-- All rights reserved.\n\nSELECT 1;\n"),
			additions: 1,
		},
		{
			name:      "HTML license comments",
			file:      sizeTestFile("", "<!-- Copyright 2026 Gravitational, Inc.\nAll rights reserved.\n-->\n<p>Hello</p>\n"),
			additions: 1,
		},
		{
			name: "build tags and package docs still count",
			file: sizeTestFile("", "//go:build linux\n\n"+apacheSizeHeader+
				"\n// Package example does useful work.\npackage example\n"),
			additions: 3,
		},
		{
			name: "Go directives adjacent to a license still count",
			file: sizeTestFile("", "//go:build linux\n// +build linux\n"+
				"// Copyright 2026 Gravitational, Inc.\n// All rights reserved.\n"+
				"//go:generate go run generate.go\n\npackage example\n"),
			additions: 4,
		},
		{
			name: "build tag after a license still counts",
			file: sizeTestFile("", "// Copyright 2026 Gravitational, Inc.\n"+
				"// All rights reserved.\n//go:build linux\n\npackage example\n"),
			additions: 2,
		},
		{
			name: "ordinary comments and license text in code still count",
			file: sizeTestFile("", "/* An ordinary comment. */\n// Copyright returns the notice.\n"+
				"const notice = `Copyright 2026`\n/* Copyright example inside code. */\n"),
			additions: 4,
		},
		{
			name:      "code on the closing comment line still counts",
			file:      sizeTestFile("", "/* Copyright 2026\n*/ const value = 1;\n"),
			additions: 1,
		},
		{
			name:      "empty and whitespace-only lines on both sides",
			file:      sizeTestFile("old()\n\n \n\t\n", "\n\t \nnew()\r\n\r\n"),
			additions: 1,
			deletions: 1,
		},
		{
			name: "existing header edits still count with context",
			file: github.PullRequestFile{Status: github.StatusModified, Additions: 1, Deletions: 1, Patch: "@@ -1,5 +1,5 @@\n" +
				" /*\n-Copyright 2025 Gravitational, Inc.\n+Copyright 2026 Gravitational, Inc.\n \n" +
				" Licensed under the Apache License, Version 2.0 (the \"License\");\n" +
				" you may not use this file except in compliance with the License."},
			additions: 1,
			deletions: 1,
		},
		{
			name: "multiple hunks and no final newline",
			file: github.PullRequestFile{Status: github.StatusModified, Additions: 3, Deletions: 2, Patch: `@@ -1,3 +1,3 @@
 /*
-Copyright 2025 Gravitational, Inc.
+Copyright 2026 Gravitational, Inc.
 */
@@ -100 +100,2 @@ func example() {
-old()
+
+new()
\ No newline at end of file`},
			additions: 2,
			deletions: 2,
		},
		{
			name:      "missing patch retains raw counts",
			file:      github.PullRequestFile{Additions: 2000, Deletions: 100},
			additions: 2000,
			deletions: 100,
		},
		{
			name:      "truncated new file patch retains unseen changes",
			file:      github.PullRequestFile{Status: github.StatusAdded, Additions: 2000, Patch: "@@ -0,0 +1,2000 @@\n+\n+/* Copyright 2026 */\n+code()"},
			additions: 1998,
		},
		{
			name:      "truncated modified file patch retains unseen changes",
			file:      github.PullRequestFile{Status: github.StatusModified, Additions: 2000, Deletions: 100, Patch: "@@ -1,100 +1,2000 @@\n-\n+\n+/* Copyright 2026 */\n+code()"},
			additions: 1999,
			deletions: 99,
		},
		{
			name:      "license in a deleted file still counts",
			file:      sizeTestFile("/* Copyright 2026 Gravitational, Inc. */\n\npackage example\n", ""),
			deletions: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.filename != "" {
				test.file.Name = test.filename
			}
			additions, deletions := changesForSizeCheck(test.file)
			require.Equal(t, test.additions, additions, "additions")
			require.Equal(t, test.deletions, deletions, "deletions")
		})
	}
}

func TestSizeCountsPreprocessorDirectives(t *testing.T) {
	const directives = `#ifndef PROJECT_METADATA_H
#define PROJECT_METADATA_H
#define PROJECT_COPYRIGHT "Copyright 2026 Example Corp."
#define PROJECT_VERSION "1.0"
#endif
`
	for _, test := range []struct {
		name    string
		content string
	}{
		{"metadata.h", directives},
		{"metadata.cpp", apacheSizeHeader + "\n" + directives},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := sizeTestFile("", test.content)
			file.Name = test.name

			additions, deletions := changesForSizeCheck(file)

			require.Equal(t, 5, additions)
			require.Zero(t, deletions)
			require.True(t, xlargeRequiresAdminApproval([]github.PullRequestFile{
				{Name: "code.go", Additions: 1499}, file,
			}))
		})
	}
}

func TestSizeExcludesBoilerplate(t *testing.T) {
	for _, test := range []struct {
		name        string
		codeLines   int
		boilerplate github.PullRequestFile
		want        sizeLabel
	}{
		{"blank additions do not cross small threshold", 99, sizeTestFile("", "\n \n\t\n"), small},
		{"license additions do not cross medium threshold", 599, sizeTestFile("", apacheSizeHeader), medium},
		{"license additions do not require admin", 1499, sizeTestFile("", agplSizeHeader), large},
		{"blank additions do not require admin", 1499, sizeTestFile("", "\n \n\t\n"), large},
		{"real additions require admin", 1500, sizeTestFile("", apacheSizeHeader), xlarge},
		{"license deletions still offset code", 1500, sizeTestFile(apacheSizeHeader, ""), large},
		{"build directives can require admin", 1499, sizeTestFile("", "// Copyright 2026 Gravitational, Inc.\n//go:build linux\n"), xlarge},
		{"blank deletions do not offset code", 1500, sizeTestFile("\n \n\t\n", ""), xlarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := []github.PullRequestFile{
				{Name: "code.go", Additions: test.codeLines},
				test.boilerplate,
			}
			require.Equal(t, test.want, prSize(files))

			changes := classifyChanges(&Config{Environment: &env.Environment{Repository: env.CoreRepo}}, files)
			require.Equal(t, test.want == xlarge, changes.Large)
		})
	}
}

// sizeTestFile creates a patch replacing the entire old contents with new contents.
func sizeTestFile(old, new string) github.PullRequestFile {
	file := github.PullRequestFile{
		Name:      "file.go",
		Status:    github.StatusModified,
		Additions: strings.Count(new, "\n"),
		Deletions: strings.Count(old, "\n"),
	}

	oldStart, newStart := 1, 1
	if old == "" {
		file.Status = github.StatusAdded
		oldStart = 0
	} else if new == "" {
		file.Status = github.StatusRemoved
		newStart = 0
	}

	var patch strings.Builder
	fmt.Fprintf(&patch, "@@ -%d,%d +%d,%d @@\n", oldStart, file.Deletions, newStart, file.Additions)

	type side struct {
		text   string
		prefix string
	}

	for _, side := range []side{{old, "-"}, {new, "+"}} {
		if side.text == "" {
			continue
		}

		for _, line := range strings.Split(strings.TrimSuffix(side.text, "\n"), "\n") {
			fmt.Fprintf(&patch, "%s%s\n", side.prefix, line)
		}
	}

	file.Patch = patch.String()

	return file
}
