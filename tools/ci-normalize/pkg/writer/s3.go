package writer

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gravitational/trace"
)

// S3Writer streams all bytes written into a single S3 object via a pipe
type S3Writer struct {
	client *s3.Client
	bucket string
	key    string

	pipeWriter *io.PipeWriter
	done       chan error
	mu         sync.Mutex
	closed     bool
}

// NewS3Writer creates a streaming writer to an S3 object
func NewS3Writer(client *s3.Client, bucket, key string) KeyedWriter {
	pr, pw := io.Pipe()
	done := make(chan error, 1)

	go func() {
		// Hardcode 20m timeout for now, if it takes longer to push the results we have some serious issues.
		ctx, cancel := context.WithTimeout(context.TODO(), time.Minute*20)
		defer cancel()

		uploader := manager.NewUploader(client)
		_, err := uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: &bucket,
			Key:    &key,
			Body:   pr,
		})
		done <- err
		close(done)
	}()

	return &S3Writer{
		client:     client,
		bucket:     bucket,
		key:        key,
		pipeWriter: pw,
		done:       done,
	}
}

// Write implements io.Writer (concurrent-safe)
func (w *S3Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, trace.BadParameter("write on closed S3Writer")
	}

	return w.pipeWriter.Write(p)
}

// Close closes the pipe and waits for upload to finish
func (w *S3Writer) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()

	w.pipeWriter.Close() // signal EOF to S3
	return <-w.done      // wait for upload to complete
}

func (w *S3Writer) SinkKey() string {
	return "s3://" + w.bucket + "/" + w.key
}

func newS3Writer(path string) (KeyedWriter, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	client := s3.NewFromConfig(cfg)

	trimmed := strings.TrimPrefix(path, "s3://")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid s3 path: %q", path)
	}
	bucket, key := parts[0], parts[1]

	return NewS3Writer(client, bucket, key), nil
}
