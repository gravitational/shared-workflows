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
package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gravitational/trace"
)

// ChangelogPR contains all the data necessary for a changelog from the PR
type ChangelogPR struct {
	Body   string `json:"body,omitempty"`
	Number int    `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`
}

// ListChangelogPullRequestsOpts contains options for searching for changelog pull requests.
type ListChangelogPullRequestsOpts struct {
	Branch   string
	FromDate string
	ToDate   string
}

// ListChangelogPullRequests will search for pull requests that provide changelog information.
func ListChangelogPullRequests(ctx context.Context, dir string, opts *ListChangelogPullRequestsOpts) ([]ChangelogPR, error) {
	var prs []ChangelogPR
	query := fmt.Sprintf("base:%s merged:%s -label:no-changelog", opts.Branch, dateRangeFormat(opts.FromDate, opts.ToDate))

	data, err := RunCmd(dir, "pr", "list", "--search", query, "--limit", "200", "--json", "number,url,title,body")
	if err != nil {
		return prs, trace.Wrap(err)
	}

	dec := json.NewDecoder(strings.NewReader(data))
	if err := dec.Decode(&prs); err != nil {
		return prs, trace.Wrap(err)
	}
	return prs, nil
}

// dateRangeFormat takes in a date range and will format it for GitHub search syntax.
// to can be empty and the format will be to search everything after from
func dateRangeFormat(from, to string) string {
	if to == "" {
		return fmt.Sprintf(">%s", from)
	}
	return fmt.Sprintf("%s..%s", from, to)
}
