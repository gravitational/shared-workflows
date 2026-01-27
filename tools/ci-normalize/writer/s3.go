// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package writer

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gravitational/trace"
)

type s3Writer struct {
	io.WriteCloser
	client *s3.Client
	writer *io.PipeWriter
	done   chan error
	mu     sync.Mutex
	closed bool
}

func (w *s3Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, io.ErrClosedPipe
	}

	return w.writer.Write(p)
}

func (w *s3Writer) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()

	_ = w.writer.Close() // signal EOF to S3
	return <-w.done      // wait for upload to complete
}

func newS3Writer(ctx context.Context, path string) (io.WriteCloser, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	client := s3.NewFromConfig(cfg)

	trimmed := strings.TrimPrefix(path, "s3://")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return nil, trace.BadParameter("invalid s3 path: %q", path)
	}
	bucket, key := parts[0], parts[1]

	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &bucket,
	}); err != nil {
		return nil, trace.Wrap(err, "unable to access s3 bucket %q", bucket)
	}

	pr, pw := io.Pipe()
	done := make(chan error, 1)

	go func() {
		defer close(done)
		uploader := manager.NewUploader(client)
		_, err := uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: &bucket,
			Key:    &key,
			Body:   pr,
		})
		done <- err
	}()

	return &s3Writer{
		client: client,
		writer: pw,
		done:   done,
	}, nil

}
