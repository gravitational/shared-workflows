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
	"log"
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/trace"
)

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

	labels, err := b.labels(ctx, files)
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

// labels determines which labels should be applied to a PR
func (b *Bot) labels(ctx context.Context, files []github.PullRequestFile) ([]string, error) {
	var labels []string

	// don't add a size label to cloud deploy PRs since they are always xl
	if !b.c.Environment.IsCloudDeployBranch() {
		labels = append(labels, string(prSize(files)))
	}

	c := classifyChanges(b.c, files)
	if c.Docs && !c.Code {
		log.Println("Label: Adding no-changelog because this is a docs-only change.")
		labels = append(labels, NoChangelogLabel)
	}

	// The branch name is unsafe, but here we are simply adding a label.
	if b.c.Environment.Repository != env.CloudRepo && isReleaseBranch(b.c.Environment.UnsafeBase) {
		log.Println("Label: Found backport branch.")
		labels = append(labels, "backport")
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name, "vendor/") {
			continue
		}

		for k, v := range prefixes[b.c.Environment.Repository] {
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

var prefixes = map[string]map[string][]string{
	env.TeleportRepo: {
		"bpf/":                {"bpf"},
		"docs/":               {"documentation"},
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
		"lib/srv/discovery":   {"discovery"},
		"lib/web/desktop.go":  {"desktop-access"},
		"tool/tctl/":          {"tctl"},
		"tool/tsh/":           {"tsh"},
		"tool/tbot/":          {"machine-id"},
		"web/":                {"ui"},
	},
	env.TeleportERepo: {
		"rfd/":             {"rfd"},
		"lib/devicetrust/": {"devicetrust"},
		"lib/idp/saml":     {"application-access", "idp"},
		"lib/loginrule/":   {"login-rules"},
		"lib/okta/":        {"application-access"},
		"lib/plugins/":     {"plugins"},
		"tool/tctl/":       {"tctl"},
		"web/":             {"ui"},
	},
	env.CloudRepo: {
		"api/":                       {"salescenter"},
		"db/salescenter":             {"salescenter", "db-migration"},
		"deploy/fluxcd/":             {"CICD"},
		"deploy/fluxcd/src/platform": {"platform"},
		"deploy/fluxcd/src/platform/helm/memcached":   {"observability"},
		"deploy/fluxcd/src/platform/helm/prometheus":  {"observability"},
		"deploy/fluxcd/src/platform/helm/promtail":    {"observability"},
		"deploy/fluxcd/src/platform/helm/pushgateway": {"observability"},
		"deploy/specs":                 {"CICD"},
		"deploy/specs/sales-center":    {"salescenter"},
		"deploy/specs/tenant-operator": {"tenant-operator"},
		"deploy/specs/prehog":          {"prehog"},
		"deploy/teleport":              {"upgrade"},
		"deploy/terraform":             {"terraform"},
		"pkg/billing":                  {"salescenter"},
		"pkg/db/":                      {"salescenter"},
		"pkg/ga-controller":            {"ga-controller"},
		"pkg/internalserver":           {"internal-server"},
		"pkg/jobs/":                    {"salescenter-job"},
		"pkg/onboarding/":              {"salescenter"},
		"pkg/onboardingserver":         {"salescenter"},
		"pkg/sales/":                   {"salescenter"},
		"pkg/salesserver":              {"salescenter"},
		"pkg/stripegateway":            {"salescenter"},
		"pkg/stripewebhook":            {"salescenter"},
		"pkg/synccontroller":           {"tenant-operator"},
		"pkg/teleportcontroller":       {"tenant-operator"},
		"pkg/tenantoperator":           {"tenant-operator"},
		"pkg/tenants/":                 {"tenant-operator"},
		"pkg/tenantsserver":            {"tenant-operator"},
		"tools/ga-controller":          {"ga-controller"},
		"tools/jobs":                   {"salescenter-job"},
		"tools/prehog":                 {"prehog"},
		"tools/salescenter":            {"salescenter"},
		"tools/tc":                     {"tc"},
		"tools/tenantoperator":         {"tenant-operator"},
		"tools/tenantsync":             {"tenant-operator"},
		"rfd/":                         {"rfd"},
		"web/":                         {"salescenter"},
	},
}
