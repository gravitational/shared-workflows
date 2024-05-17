package checker

import "context"

type CodeChecker interface {
	ShouldCheckForVulnerabilities(prChangedFilePaths []string) bool
	DoCheck(ctx context.Context, localChangedFilePaths []string) error
}
