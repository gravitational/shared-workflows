package fileutil

import (
	"io"
	"os"

	"github.com/gravitational/trace"
)

// CopyFile copies a file from src to dst.
func CopyFile(src, dst string) error {
	w, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755) // create or overwrite
	if err != nil {
		return trace.Wrap(err)
	}
	defer w.Close()

	r, err := os.OpenFile(src, os.O_RDONLY, 0)
	if err != nil {
		return trace.Wrap(err)
	}
	defer r.Close()

	if _, err = io.Copy(w, r); err != nil {
		return trace.Wrap(err)
	}

	return nil
}
