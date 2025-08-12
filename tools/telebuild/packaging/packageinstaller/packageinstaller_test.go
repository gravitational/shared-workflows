/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package packageinstaller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gravitational/shared-workflows/tools/telebuild/internal/exec"
	"github.com/stretchr/testify/require"
)

// Test only runs on macOS and requires root privileges.
func TestPackageInstallerPackager(t *testing.T) {
	// TestPackageInstaller tests the package installer.
	// It installs a package and checks that the expected files are installed.
	// It then uninstalls the package and checks that the expected files are removed.
	bundleID := "com.gravitational.testingpackage"

	tempOutdir := t.TempDir()
	tempInstallDir := t.TempDir()

	// Create a new package installer packager.
	pkg, err := NewPackager(
		Info{
			BundleID:        bundleID,
			Version:         "1.0",
			InstallLocation: tempInstallDir,
			RootPath:        "testdata/packageinstaller",
			OutputPath:      filepath.Join(tempOutdir, "packageinstaller.pkg"),
		},
	)
	require.NoError(t, err)

	err = pkg.Package()
	require.NoError(t, err)

	// Check that the package was created.
	_, err = os.Stat(pkg.Info.OutputPath)
	require.NoError(t, err)

	// Install the package.
	runner := exec.NewDefaultCommandRunner()
	out, err := runner.RunCommand("installer", "-pkg", pkg.Info.OutputPath, "-target", "/")
	require.NoError(t, err, out)

	// Check that receipt exists
	out, err = runner.RunCommand("pkgutil", "--pkg-info", bundleID)
	require.NoError(t, err, out)

	// Check that the fakebinary exists
	_, err = os.Stat(filepath.Join(tempInstallDir, "fakebinary"))
	require.NoError(t, err)

	// Remove package from system.
	out, err = runner.RunCommand("pkgutil", "--forget", bundleID)
	require.NoError(t, err, out)
}
