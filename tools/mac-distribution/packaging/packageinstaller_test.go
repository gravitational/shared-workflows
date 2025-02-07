package packaging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gravitational/shared-workflows/tools/mac-distribution/internal/exec"
	"github.com/stretchr/testify/require"
)

// Test only runs on macOS and requires root privileges.
func TestPackageInstallerPackager(t *testing.T) {
	// TestPackageInstaller tests the package installer.
	// It installs a package and checks that the expected files are installed.
	// It then uninstalls the package and checks that the expected files are removed.
	bundleID := "com.gravitational.testingpackage"

	testdataPath, err := filepath.Abs("testdata")
	require.NoError(t, err)
	tempOutdir, err := os.MkdirTemp(testdataPath, "pkgstaging")
	require.NoError(t, err)
	defer os.RemoveAll(tempOutdir)

	tempInstallDir, err := os.MkdirTemp(testdataPath, "pkginstall")
	require.NoError(t, err)
	defer os.RemoveAll(tempInstallDir)

	// Create a new package installer packager.
	pkg := NewPackageInstallerPackager(
		&PackageInstallerInfo{
			BundleID:        bundleID,
			Version:         "1.0",
			InstallLocation: tempInstallDir,
			RootPath:        "testdata/packageinstaller",
			OutputPath:      filepath.Join(tempOutdir, "packageinstaller.pkg"),
		},
		&PackageInstallerPackagerOpts{},
	)

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
