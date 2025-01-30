package notarize

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type zipper interface {
	ZipDir(dir string, out io.Writer) error
}

type dirZipper struct {
	// IncludePrefix determines whether to keep the root directory as a prefix in the zip file.
	// This is particularly useful for App Bundles where the root directory (.app) should be included.
	IncludePrefix bool
}

// zipDir will zip the directory into the specified output file
func (z *dirZipper) ZipDir(dir string, out io.Writer) error {
	zipwriter := zip.NewWriter(out)
	defer zipwriter.Close()

	root := filepath.Clean(dir)

	filepath.WalkDir(root, fs.WalkDirFunc(func(path string, d fs.DirEntry, err error) error {
		// Ignore zipping directories
		if d.IsDir() {
			return nil
		}

		if !z.IncludePrefix {
			path, _ = strings.CutPrefix(path, root+string(os.PathSeparator))
		}

		w, err := zipwriter.Create(path)
		if err != nil {
			return err
		}

		f, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(w, f)
		return err
	}))
	return nil
}
