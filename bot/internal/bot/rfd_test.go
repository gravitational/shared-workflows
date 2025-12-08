package bot

import (
	"context"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/stretchr/testify/require"
)

func TestValidateNewRFD(t *testing.T) {

	tests := []struct {
		desc         string
		branch       string
		files        []github.PullRequestFile
		errorMessage string
	}{
		{
			desc:   "code-only",
			branch: "foo",
			files: []github.PullRequestFile{
				{
					Name:   "file.go",
					Status: github.StatusAdded,
				},
				{
					Name:   "examples/README.md",
					Status: github.StatusChanged,
				},
			},
		},
		{
			desc:   "valid-rfd",
			branch: "rfd/0001-test-123",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/0001-test-123.md",
					Status: github.StatusAdded,
				},
			},
		},
		{
			desc:   "rfd-branch-assets-only",
			branch: "rfd/0001-test-123",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/assets/0001-test-123.png",
					Status: github.StatusAdded,
				},
			},
			errorMessage: `RFD "rfd/0001-test-123.md" is missing`,
		},
		{
			desc:   "random-branch-rfd-assets-only",
			branch: "rjones/test-123",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/assets/0001-test-123.png",
					Status: github.StatusAdded,
				},
			},
		},
		{
			desc:   "invalid-rfd-branch",
			branch: "rjones/rfd-0001-test-123",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/0001-test-123.md",
					Status: github.StatusAdded,
				},
			},
			errorMessage: "RFD branches must follow the pattern rfd/$number-your-title",
		},
		{
			desc:   "invalid-rfd-name",
			branch: "rfd/0001-test-123",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/test-123.md",
					Status: github.StatusAdded,
				},
			},
			errorMessage: `Found RFD named "rfd/test-123.md", expected RFD to be named "rfd/0001-test-123.md"`,
		},
		{
			desc:   "missing-rfd",
			branch: "rfd/0001-test-123",
			files: []github.PullRequestFile{
				{
					Name:   "file.go",
					Status: github.StatusAdded,
				},
				{
					Name:   "examples/README.md",
					Status: github.StatusAdded,
				},
			},
			errorMessage: `RFD "rfd/0001-test-123.md" is missing`,
		},
		{
			desc:   "deleting-rfd",
			branch: "rjones/remove_rfd",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/0001-test-123.md",
					Status: github.StatusRemoved,
				},
			},
		},
		{
			desc:   "copied-rfd",
			branch: "rjones/remove_rfd",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/0001-test-123.md",
					Status: github.StatusCopied,
				},
			},
		},
		{
			desc:   "modified-rfd",
			branch: "rjones/remove_rfd",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/0001-test-123.md",
					Status: github.StatusModified,
				},
			},
		},
		{
			desc:   "renamed-rfd",
			branch: "rjones/remove_rfd",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/0001-test-123.md",
					Status: github.StatusRenamed,
				},
			},
		},
		{
			desc:   "multiple-rfds",
			branch: "rfd/0001-test-123",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/0001-test-123.md",
					Status: github.StatusAdded,
				},
				{
					Name:   "rfd/0002-test-124.md",
					Status: github.StatusAdded,
				},
			},
			errorMessage: `Found RFD named "rfd/0002-test-124.md", expected RFD to be named "rfd/0001-test-123.md"`,
		},
		{
			desc:   "rfd-number-padding",
			branch: "rfd/1-test-123",
			files: []github.PullRequestFile{
				{
					Name:   "rfd/1-test-123.md",
					Status: github.StatusAdded,
				},
			},
			errorMessage: `Found branch named "rfd/1-test-123", expected branch to be named "rfd/0001-test-123"`,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			b := &Bot{
				c: &Config{
					Environment: &env.Environment{
						Organization: "foo",
						Repository:   "test",
						UnsafeHead:   test.branch,
					},
					GitHub: &fakeGithub{
						files: test.files,
					},
				},
			}

			err := b.ValidateNewRFD(context.Background())
			if test.errorMessage == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, test.errorMessage)
		})
	}
}
