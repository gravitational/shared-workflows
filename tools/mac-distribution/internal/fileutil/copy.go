package fileutil

import (
	"io"
	"os"

	"github.com/gravitational/trace"
)

// CopyOpt is a functional option for configuring the copy operation.
type CopyOpt func(*copyOpts)

type copyOpts struct {
	destPermissions os.FileMode
}

// CopyFile copies a file from src to dst.
func CopyFile(src, dst string, opts ...CopyOpt) (err error) {
	var o copyOpts
	for _, opt := range opts {
		opt(&o)
	}

	r, err := os.Open(src)
	if err != nil {
		return trace.Wrap(err)
	}
	defer r.Close()

	// If the destination permissions are not set, use the source permissions.
	if o.destPermissions == 0 {
		info, err := r.Stat()
		if err != nil {
			return trace.Wrap(err)
		}
		o.destPermissions = info.Mode().Perm()
	}

	w, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, o.destPermissions) // create or overwrite
	if err != nil {
		return trace.Wrap(err)
	}

	// Close the writer and remove the destination file if an error occurs.
	defer func() {
		closeErr := w.Close()
		if err == nil { // return close error if NO ERROR occurred before
			err = closeErr
		}
		if err != nil {
			// Attempt to remove the destination file if an error occurred.
			err = trace.NewAggregate(err, trace.Wrap(os.Remove(dst), "failed to remove destination file"))
		}
	}()

	if _, err = io.Copy(w, r); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// WithDestPermissions sets the permissions of the destination file.
// By default the destination file will have the same permissions as the source file.
func WithDestPermissions(perm os.FileMode) CopyOpt {
	return func(o *copyOpts) {
		o.destPermissions = perm
	}
}
