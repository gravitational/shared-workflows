package main

import (
	"context"
	"fmt"
	"os"

	kingpin "github.com/alecthomas/kingpin/v2"
	"golang.org/x/sync/errgroup"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/dispatch"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/dispatch/adapter"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/encoder"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/input"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/meta"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/writer"
	"github.com/gravitational/trace"
)

func makeWriter[T any](ctx context.Context, paths []string, format string, metadata *record.Meta) ([]dispatch.Option, error) {
	var opts []dispatch.Option
	for _, path := range paths {
		raw, err := writer.New(ctx, path, metadata)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		var enc encoder.Encoder
		switch format {
		case "jsonl":
			enc = encoder.NewJSONLEncoder(raw)
		default:
			return nil, trace.BadParameter("unsupported format %q", format)
		}

		opts = append(opts, dispatch.WithWriter(new(T), adapter.New(enc, raw)))
	}
	return opts, nil
}

func setupDispatcher(ctx context.Context, format string, metadata *record.Meta, suiteOuts, testOuts, metaOuts []string) (*dispatch.Dispatcher, error) {
	opts := []dispatch.Option{}

	suiteOpts, err := makeWriter[record.Suite](ctx, suiteOuts, format, metadata)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	opts = append(opts, suiteOpts...)

	testOpts, err := makeWriter[record.Testcase](ctx, testOuts, format, metadata)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	opts = append(opts, testOpts...)

	metaOpts, err := makeWriter[record.Meta](ctx, metaOuts, format, metadata)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	opts = append(opts, metaOpts...)

	d, err := dispatch.New(ctx, opts...)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return d, nil
}

func createProducers(cmd string, junitCmd *kingpin.CmdClause, metadata *record.Meta, junitFiles *[]string) ([]input.Producer, error) {
	producers := []input.Producer{input.NewPassthroughProducer(metadata)}
	opts := []input.Option{input.WithMeta(metadata)}

	switch cmd {
	case junitCmd.FullCommand():
		for _, f := range *junitFiles {
			p, err := input.NewJUnitProducer(f, opts...)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			producers = append(producers, p)
		}
	default:
		return nil, trace.NotImplemented("unimplemented command %q", cmd)
	}

	if len(producers) == 0 {
		return nil, trace.BadParameter("nothing to do")
	}

	return producers, nil
}

func run() error {
	ctx := context.Background()
	app := kingpin.New("ci-normalize", "Normalize test artifacts")
	app.HelpFlag.Short('h')
	format := app.Flag("format", "Output format").Short('f').Default("jsonl").Enum("jsonl")
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

	dispatcher, err := setupDispatcher(ctx, *format, metadata, *suiteOuts, *testOuts, *metaOuts)
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
	for _, p := range producers {
		p := p // capture
		eg.Go(func() error {
			return p.Produce(ctx, dispatcher.Write)
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
