/*
Copyright 2021 Gravitational, Inc.

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
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/trace"
)

const backportLabelFormat = "backport/branch/v%v"
const supportedPreviousVersions = 3 // Includes the current major release

// Label parses the content of the PR (branch name, files, etc) and sets
// appropriate labels.
func (b *Bot) Label(ctx context.Context) error {
	files, err := b.c.GitHub.ListFiles(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number)
	if err != nil {
		return trace.Wrap(err)
	}

	v, err := b.c.GitHub.GetLatestReleaseMajorVersion(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository)
	if err != nil {
		return trace.Wrap(err)
	}

	labels, err := b.labels(ctx, files, v)
	if err != nil {
		return trace.Wrap(err)
	}
	if len(labels) == 0 {
		return nil
	}

	err = b.c.GitHub.AddLabels(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
		labels)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// labels determines which labels should be applied to a PR given the current PR
// files and major version of the latest Teleport release.
func (b *Bot) labels(ctx context.Context, files []github.PullRequestFile, majorVersion int) ([]string, error) {
	var labels []string

	labels = append(labels, string(prSize(files)))

	// The branch name is unsafe, but here we are simply adding a label.
	if isReleaseBranch(b.c.Environment.UnsafeBase) {
		log.Println("Label: Found backport branch.")
		labels = append(labels, "backport")
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name, "vendor/") {
			continue
		}

		docs, _, err := classifyChanges(b.c.Environment, files)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		if docs {
			labels = append(labels, "documentation")

			// For docs PRs, attach backport labels for the current
			// major version and the versions we currently support.
			// This way, we can always remember to backport relevant
			// docs changes.
			for i := 0; i < supportedPreviousVersions && majorVersion-i > 0; i++ {
				labels = append(labels, fmt.Sprintf(backportLabelFormat, majorVersion-i))
			}
		}

		for k, v := range prefixes {
			if strings.HasPrefix(file.Name, k) {
				log.Printf("Label: Found prefix %v, attaching labels: %v.", k, v)
				labels = append(labels, v...)
			}
		}
	}

	return deduplicate(labels), nil
}

func deduplicate(s []string) []string {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}

	var out []string
	for k := range m {
		out = append(out, k)
	}

	return out
}

var prefixes = map[string][]string{
	"bpf/":                {"bpf"},
	"rfd/":                {"rfd"},
	"examples/chart":      {"helm"},
	"lib/bpf/":            {"bpf"},
	"lib/events":          {"audit-log"},
	"lib/kube":            {"kubernetes-access"},
	"lib/tbot/":           {"machine-id"},
	"lib/srv/desktop":     {"desktop-access"},
	"lib/srv/desktop/rdp": {"desktop-access", "rdp"},
	"lib/srv/app/":        {"application-access"},
	"lib/srv/db":          {"database-access"},
	"lib/web/desktop.go":  {"desktop-access"},
	"tool/tctl/":          {"tctl"},
	"tool/tsh/":           {"tsh"},
	"tool/tbot/":          {"machine-id"},
	"web/":                {"ui"},
}
