package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/stretchr/testify/require"
)

func TestValidateManualTestPlan(t *testing.T) {
	tests := []struct {
		desc      string
		body      string
		labels    []string
		assertion require.ErrorAssertionFunc
	}{
		{
			desc:      "pass-no-test-plan-label",
			labels:    []string{"no-test-plan"},
			assertion: require.NoError,
		},
		{
			desc:      "fail-other-labels",
			labels:    []string{"no-changelog", "backport/branch/v18"},
			assertion: require.Error,
		},
		{
			desc: "pass-valid-test-plan",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"",
				"\r",
				"\n",
				"\t",
				"",
				"### Test Environment",
				"",
				"rjones.cloud.gravitational.io",
				"",
				"### Test Cases",
				"- [x] Verify login works",
				"- [x] Verify logout works",
			}, "\n"),
			assertion: require.NoError,
		},
		{
			desc: "pass-case-insensitive-test-case",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"### Test Environment",
				"",
				"staging",
				"",
				"### Test Cases",
				"- [X] Verify login works",
			}, "\n"),
			assertion: require.NoError,
		},
		{
			desc: "pass-section-followed-by-another-heading",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"### Test Environment",
				"",
				"production",
				"",
				"### Test Cases",
				"- [x] Step one",
				"- [x] Step two",
				"## Another Section",
				"Some other content",
			}, "\n"),
			assertion: require.NoError,
		},
		{
			desc: "pass-section-with-extra-content-before",
			body: strings.Join([]string{
				"## Summary",
				"This is a summary.",
				"## Manual Test Plan",
				"",
				"### Test Environment",
				"",
				"dev",
				"",
				"### Test Cases",
				"- [x] First check",
				"- [x] Second check",
			}, "\n"),
			assertion: require.NoError,
		},
		{
			desc:      "fail-missing-section",
			body:      "## Summary\nSome description without a test plan.",
			assertion: require.Error,
		},
		{
			desc: "fail-missing-environment",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"### Test Cases",
				"- [x] Verify login works",
			}, "\n"),
			assertion: require.Error,
		},
		{
			desc: "fail-empty-environment",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"### Test Environment",
				"   ",
				"### Test Cases",
				"- [x] Verify login works",
			}, "\n"),
			assertion: require.Error,
		},
		{
			desc: "fail-missing-test-cases-label",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"### Test Environment",
				"",
				"staging",
				"",
				"- [x] Verify login works",
			}, "\n"),
			assertion: require.Error,
		},
		{
			desc: "fail-no-test-casees",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"### Test Environment",
				"",
				"staging",
				"",
				"### Test Cases",
				"Just some text without any test-casees.",
			}, "\n"),
			assertion: require.Error,
		},
		{
			desc: "fail-unchecked-test-case",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"### Test Environment",
				"",
				"staging",
				"",
				"### Test Cases",
				"- [x] Verify login works",
				"- [ ] Verify logout works",
			}, "\n"),
			assertion: require.Error,
		},
		{
			desc: "fail-all-unchecked",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"### Test Environment",
				"",
				"staging",
				"",
				"### Test Cases",
				"- [ ] Verify login works",
				"- [ ] Verify logout works",
			}, "\n"),
			assertion: require.Error,
		},
		{
			desc: "fail-test-case-outside-section-not-counted",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"### Test Environment",
				"",
				"staging",
				"",
				"### Test Cases",
				"## Another Section",
				"- [x] This test-case is outside the test plan section",
			}, "\n"),
			assertion: require.Error,
		},
		{
			desc: "pass-no-space-between-items",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"### Test Environment",
				"staging",
				"### Test Cases",
				"- [X] Verify login works",
				"- [x] Verify logout works",
			}, "\n"),
			assertion: require.NoError,
		},
		{
			desc: "pass-extra-space-between-items",
			body: strings.Join([]string{
				"## Manual Test Plan",
				"",
				"\t",
				"",
				"### Test Environment",
				"",
				"staging",
				"\t",
				"",
				"",
				"### Test Cases",
				"- [X] Verify login works",
				"- [x] Verify logout works",
			}, "\n"),
			assertion: require.NoError,
		},
		{
			desc: "pass-full-pr-body",
			body: strings.Join([]string{
				"Mollitia at voluptas error labore. Quia adipisci corrupti iure. Odio fugiat eligendi cumque. Saepe voluptatem soluta et ipsa minima quas.",
				"",
				"Ea sunt et a. Labore tempore sequi dolores adipisci eaque voluptatem. Aliquid earum laboriosam aut suscipit perspiciatis et maxime repellendus. Voluptatibus est cumque error ut vel. Veniam eveniet rerum et ut eum.",
				"",
				"## Manual Test Plan",
				"",
				"\t",
				"",
				"### Test Environment",
				"",
				"staging",
				"\t",
				"",
				"",
				"### Test Cases",
				"- [X] Verify login works",
				"- [x] Verify logout works",
			}, "\n"),
			assertion: require.NoError,
		},
		{
			desc:      "fail-empty-body",
			body:      "",
			assertion: require.Error,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			b := &Bot{
				c: &Config{
					Environment: &env.Environment{
						Organization: "foo",
						Author:       "baz",
						Repository:   "bar",
						Number:       0,
						UnsafeBase:   "branch/v18",
						UnsafeHead:   "fix",
					},
					GitHub: &fakeGithub{
						comments: []github.Comment{
							{
								Author: "foo@bar.com",
								Body:   "PR comment body",
							},
						},
						pull: github.PullRequest{
							UnsafeBody:   test.body,
							UnsafeLabels: test.labels,
						},
					},
				},
			}

			test.assertion(t, b.ValidateManualTestPlan(context.Background()))
		})
	}
}
