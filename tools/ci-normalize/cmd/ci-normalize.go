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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	kingpin "github.com/alecthomas/kingpin/v2"
	"golang.org/x/sync/errgroup"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/dispatch"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/input"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/meta"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/writer"
	"github.com/gravitational/trace"
)

func makeWriters(
	ctx context.Context,
	paths []string,
	metadata *record.Meta,
) ([]dispatch.RecordWriter, error) {

	var writers []dispatch.RecordWriter

	for _, path := range paths {
		w, err := writer.New(ctx, path, metadata, func(w io.Writer) writer.Encoder {
			return json.NewEncoder(w)
		})
		if err != nil {
			return nil, trace.Wrap(err)
		}

		writers = append(writers, w)
	}

	return writers, nil
}

func setupDispatcher(
	ctx context.Context,
	metadata *record.Meta,
	suiteOuts, testOuts, metaOuts []string,
) (*dispatch.Dispatcher, error) {

	suiteWriters, err := makeWriters(ctx, suiteOuts, metadata)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	testWriters, err := makeWriters(ctx, testOuts, metadata)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	metaWriters, err := makeWriters(ctx, metaOuts, metadata)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	d, err := dispatch.New(
		ctx,
		suiteWriters,
		testWriters,
		metaWriters,
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return d, nil
}
func createProducers(cmd string, junitCmd *kingpin.CmdClause, metadata *record.Meta, junitFiles *[]string) ([]input.Producer, error) {
	producers := []input.Producer{}
	switch cmd {
	case junitCmd.FullCommand():
		for _, f := range *junitFiles {
			p, err := input.NewJUnitProducer(f, metadata)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			producers = append(producers, p)
		}
	default:
		return nil, trace.NotImplemented("unimplemented command %q", cmd)
	}

	return producers, nil
}

func run() error {
	ctx := context.Background()
	app := kingpin.New("ci-normalize", "Normalize test artifacts")
	app.HelpFlag.Short('h')
	timeout := app.Flag(
		"timeout",
		"Maximum execution time (e.g. 30s, 2m); 0 means no timeout",
	).Default("0").Duration()

	// JUnit command
	junitCmd := app.Command("junit", "Normalize JUnit test results")
	suiteOuts := junitCmd.Flag("suites", "Testsuite output(s) ('-' for stdout, /dev/null to ignore)").Default("-").Strings()
	testOuts := junitCmd.Flag("tests", "Testcase output(s) ('-' for stdout, /dev/null to ignore)").Default("-").Strings()
	junitFiles := junitCmd.Arg("files", "JUnit XML result files").Required().ExistingFiles()
	metaFile := junitCmd.Flag("from-meta", "Optionally provide existing metadata").ExistingFile()
	metaOuts := junitCmd.Flag(
		"meta",
		"Metadata output ('-' for stdout, /dev/null to ignore)",
	).Short('o').Default("-").Strings()

	cmd, err := app.Parse(os.Args[1:])
	if err != nil {
		return trace.Wrap(err, "failed to parse command line arguments")

	}

	// setup timeout context
	var cancel context.CancelFunc
	if *timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, *timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	metadata, err := meta.New(metaFile)
	if err != nil {
		return trace.Wrap(err, "reading metadata")
	}

	dispatcher, err := setupDispatcher(ctx, metadata, *suiteOuts, *testOuts, *metaOuts)
	if err != nil {
		return trace.Wrap(err)
	}
	defer func() {
		// Clean up path, ignore the err
		_ = dispatcher.Close()
	}()

	producers, err := createProducers(cmd, junitCmd, metadata, junitFiles)
	if err != nil {
		return trace.Wrap(err)
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		// Always emit metadata record
		return dispatcher.WriteMeta(metadata)
	})

	for _, p := range producers {
		eg.Go(func() error {
			return p.Produce(ctx, dispatcher)
		})
	}

	if err := eg.Wait(); err != nil {
		return trace.Wrap(err)
	}

	return dispatcher.Close() // flush and check for errors
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
