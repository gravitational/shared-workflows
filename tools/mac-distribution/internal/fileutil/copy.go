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

package fileutil

import (
	"fmt"
	"io"
	"os"

	"errors"
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
		return err
	}
	defer r.Close()

	// If the destination permissions are not set, use the source permissions.
	if o.destPermissions == 0 {
		info, err := r.Stat()
		if err != nil {
			return err
		}
		o.destPermissions = info.Mode().Perm()
	}

	w, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, o.destPermissions) // create or overwrite
	if err != nil {
		return err
	}

	// Close the writer and remove the destination file if an error occurs.
	defer func() {
		closeErr := w.Close()
		if err == nil { // return close error if NO ERROR occurred before
			err = closeErr
		}
		if err != nil {
			// Attempt to remove the destination file if an error occurred.
			rmErr := os.Remove(dst)
			if rmErr != nil {
				err = errors.Join(err, fmt.Errorf("failed to remove destination file: %w", rmErr))
			}
		}
	}()

	if _, err = io.Copy(w, r); err != nil {
		return err
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
