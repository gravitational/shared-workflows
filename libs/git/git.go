/*
Copyright 2024 Gravitational, Inc.

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

package git

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/gravitational/trace"
)

// Repo wraps a local git repository and runs git commands against it.
type Repo struct {
	dir string
}

// NewRepo initializes [Repo] from a directory.
func NewRepo(dir string) *Repo {
	return &Repo{dir: dir}
}

// ObjectSHAAtPath returns the SHA of the object at path as of ref,
// e.g. the commit a submodule points to.
func (r *Repo) ObjectSHAAtPath(ref, path string) (string, error) {
	sha, err := r.RunCmd("rev-parse", ref+":"+path)
	if err != nil {
		return "", trace.Wrap(err, "can't get object SHA for ref %q, path %q", ref, path)
	}
	return sha, nil
}

// prRegex matches the "(#N)" suffix GitHub squash merges append to commit subjects.
var prRegex = regexp.MustCompile(`\(#(\d+)\)\s*$`)

// PRsBetweenRefs returns the pull request numbers referenced by commits in
// baseRef..headRef. A commit references a PR when its subject ends with
// "(#N)", as produced by GitHub squash merges; commits without a PR
// reference are skipped.
func (r *Repo) PRsBetweenRefs(baseRef, headRef string) ([]int, error) {
	commits, err := r.RunCmd("log", "--format=%s", baseRef+".."+headRef)
	if err != nil {
		return nil, trace.Wrap(err, "can't get commits between refs %q and %q", baseRef, headRef)
	}

	var prNumbers []int
	for _, commit := range strings.Split(commits, "\n") {
		matches := prRegex.FindStringSubmatch(commit)
		if matches == nil {
			continue
		}
		n, err := strconv.Atoi(matches[1])
		if err != nil {
			continue // digits too long to be a PR number
		}
		prNumbers = append(prNumbers, n)
	}

	return prNumbers, nil
}
