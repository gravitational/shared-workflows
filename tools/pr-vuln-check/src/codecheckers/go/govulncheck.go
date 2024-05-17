package gochecker

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/gravitational/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/tools/go/packages"
	"golang.org/x/vuln/scan"
)

type GoChecker struct {
	forceRunPaths map[string]string
}

func NewGoChecker(forceRunPaths map[string]string) *GoChecker {
	return &GoChecker{
		forceRunPaths: forceRunPaths,
	}
}

func (gc *GoChecker) ShouldCheckForVulnerabilities(prChangedFilePaths []string) bool {
	forcedRunPaths := maps.Keys(gc.forceRunPaths)

	for _, changedFilePath := range prChangedFilePaths {
		if slices.Contains(forcedRunPaths, changedFilePath) {
			return true
		}

		if filepath.Ext(changedFilePath) == ".go" {
			return true
		}

		fileName := filepath.Base(changedFilePath)
		if fileName == "go.mod" || fileName == "go.sum" {
			return true
		}
	}

	log.Println("PR contains no Go code changes, and matched no Go force run paths")
	return false
}

func (gc *GoChecker) DoCheck(ctx context.Context, localChangedFilePaths []string) error {
	goModuleDirectories, err := gc.getDirectoriesToCheck(ctx, localChangedFilePaths)
	if err != nil {
		return trace.Wrap(err, "failed to get all Go module directories to check")
	}

	return gc.checkGoModulesForVulnerabilities(ctx, goModuleDirectories)
}

// Takes in a list of changed file paths in a PR, and returns a list of Go module directories that need to be
// checked for vulnerabilities.
func (gc *GoChecker) getDirectoriesToCheck(ctx context.Context, changedFilePaths []string) ([]string, error) {
	goModDirectories := make([]string, 0)
	for _, changedFilePath := range changedFilePaths {
		goModDirectory, err := gc.getModDirectoryForPath(ctx, changedFilePath)
		if err != nil {
			return nil, trace.Wrap(err, "failed to load Go mod directory for path %q", changedFilePath)
		}

		if slices.Contains(goModDirectories, goModDirectory) {
			continue
		}

		log.Printf("Found a module at %q via file at %q", goModDirectory, changedFilePath)
		goModDirectories = append(goModDirectories, goModDirectory)
	}

	return goModDirectories, nil
}

// Takes in a path to an arbitrary file, and attempts to return the top-level Go module path associated
// with the file.
func (gc *GoChecker) getModDirectoryForPath(ctx context.Context, filePath string) (string, error) {
	fileName := filepath.Base(filePath)
	if fileName == "go.mod" || fileName == "go.sum" {
		return gc.getModDirectoryForGoModFile(filePath), nil
	}

	if filepath.Ext(filePath) == ".go" {
		goModDirectory, err := gc.getModDirectoryForGoSourceFile(ctx, filePath)
		if err != nil {
			return "", trace.Wrap(err, "failed to find top-level Go module directory for Go source file")
		}

		return goModDirectory, nil
	}

	goModDirectory, err := gc.getModDirectoryForNonGoFile(filePath)
	if err != nil {
		return "", trace.Wrap(err, "failed to find top-level Go module directory for non-Go file")
	}

	return goModDirectory, nil
}

// Takes in the path to a "go.mod" or "go.sum" file, and outputs the top-level module directory containing the file.
func (gc *GoChecker) getModDirectoryForGoModFile(filePath string) string {
	return filepath.Dir(filePath)
}

// Takes in the path to a "*.go" source file, and outputs the top-level module directory containing the file.
func (gc *GoChecker) getModDirectoryForGoSourceFile(ctx context.Context, filePath string) (string, error) {
	packagesLoadConfig := &packages.Config{
		Mode:    packages.NeedModule,
		Context: ctx,
	}

	packages, err := packages.Load(packagesLoadConfig, "file="+filePath)
	if err != nil {
		return "", trace.Wrap(err, "failed to load packages for file %q", filePath)
	}

	if len(packages) == 0 {
		return "", trace.Wrap(err, "failed to load the package containing the Go source file at %q", filePath)
	}

	// Returning the module for the first package, under the assumption that packages that enclose the source file
	// must always be within the same module.
	return packages[0].Module.Dir, nil
}

// Takes in a non-Go source file and attempts to find the associated top-level module directory containing the file.
func (gc *GoChecker) getModDirectoryForNonGoFile(filePath string) (string, error) {
	if forceRunDirectory, ok := gc.forceRunPaths[filePath]; ok {
		return forceRunDirectory, nil
	}

	// TODO maybe add some logic to walk upwards at some point
	return "", trace.Errorf("failed to find top-level Go module directory for file path %q", filePath)
}

func (gc *GoChecker) checkGoModulesForVulnerabilities(ctx context.Context, goModDirectories []string) error {
	for _, goModDirectory := range goModDirectories {
		log.Printf("Checking module at %q...", goModDirectory)
		cmd := scan.Command(ctx, "-C", goModDirectory, "-show", "verbose,traces", "./...")
		cmd.Env = append(os.Environ(), "GOWORK=off")

		err := cmd.Start()
		if err != nil {
			return trace.Wrap(err, "failed to start the govulncheck for module at %q", goModDirectory)
		}

		err = cmd.Wait()
		if err != nil {
			return trace.Wrap(err, "an error occurred while running govulncheck")
		}
	}

	return nil
}
