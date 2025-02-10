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

// FileInfo contains information about a file to archive.
type FileInfo struct {
	// Path is the path to the file.
	Path string

	// ArchiveName is the desired name of the file in the archive.
	// If ArchiveName is empty, the base name of path will be used.
	ArchiveName string
}

// ZipFiles will create a zip with the specified files and write the archive to the specified writer.
func ZipFiles(out io.Writer, files []FileInfo) (err error) {
	zipwriter := zip.NewWriter(out)
	defer func() {
		if err == nil { // if NO errors
			// Closing finishes the write by writing the central directory.
			// To avoid propagating an error from an earlier operation only close if there is no error.
			err = trace.Wrap(zipwriter.Close())
		}
	}()

	for _, file := range files {
		if file.ArchiveName == "" {
			file.ArchiveName = filepath.Base(file.Path)
		}

		w, err := zipwriter.Create(file.ArchiveName)
		if err != nil {
			return trace.Wrap(err)
		}

		f, err := os.Open(file.Path)
		if err != nil {
			return trace.Wrap(err)
		}

		_, err = io.Copy(w, f)
		if err != nil {
			f.Close() // Ignore close error since we already have an error to return.
			return trace.Wrap(err)
		}

		if err := f.Close(); err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

// DirZipperOpt is a functional option for configuring a DirZipper.
type DirZipperOpt func(*dirZipperOpts)

type dirZipperOpts struct {
	includeParent bool
}

// IncludeParent determines whether to keep the root directory as a prefix in the zip file.
// This is particularly useful for App Bundles where the root directory (.app) should be included.
func IncludeParent() DirZipperOpt {
	return func(o *dirZipperOpts) {
		o.includeParent = true
	}
}

// ZipDir will zip the directory into the specified output file
func ZipDir(dir string, out io.Writer, opts ...DirZipperOpt) (err error) {
	var o dirZipperOpts
	for _, opt := range opts {
		opt(&o)
	}

	files := []FileInfo{}
	parentDir := filepath.Base(dir)

	// Construct a list of files to include in the zip
	err = filepath.WalkDir(dir, fs.WalkDirFunc(func(path string, d fs.DirEntry, err error) error {
		// Ignore zipping directories
		if d.IsDir() {
			return nil
		}

		// Avoid including root path structure in the zip file.
		archiveName := strings.TrimPrefix(path, dir)

		if o.includeParent {
			archiveName = filepath.Join(parentDir, archiveName)
		}

		files = append(files, FileInfo{
			Path:        path,
			ArchiveName: archiveName,
		})
		return nil
	}))

	return trace.Wrap(ZipFiles(out, files))
}
