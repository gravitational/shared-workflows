package zipper

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitational/trace"
)

type DirZipper interface {
	ZipDir(dir string, out io.Writer, opts DirZipperOpts) error
}

type DirZipperOpts struct {
	// IncludePrefix determines whether to keep the root directory as a prefix in the zip file.
	// This is particularly useful for App Bundles where the root directory (.app) should be included.
	IncludePrefix bool
}

func NewDirZipper() DirZipper {
	return &DefaultZipper{}
}

// DefaultZipper is the default implementation of DirZipper
type DefaultZipper struct{}

// ZipDir will zip the directory into the specified output file
func (z *DefaultZipper) ZipDir(dir string, out io.Writer, opts DirZipperOpts) error {
	zipwriter := zip.NewWriter(out)
	defer zipwriter.Close()

	root := filepath.Clean(dir)

	err := filepath.WalkDir(root, fs.WalkDirFunc(func(path string, d fs.DirEntry, err error) error {
		// Ignore zipping directories
		if d.IsDir() {
			return nil
		}

		if !opts.IncludePrefix {
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
	return trace.Wrap(err)
}
