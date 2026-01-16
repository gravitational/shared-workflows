package main

import (
	"context"
	"fmt"
	"os"

	kingpin "github.com/alecthomas/kingpin/v2"
	"golang.org/x/sync/errgroup"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/dispatch"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/dispatch/adapter"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/encoder"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/input"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/meta"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/writer"
	"github.com/gravitational/trace"
)

func makeDefaultWriter(format *string) (dispatch.RecordWriter, error) {
	defaultRaw, err := writer.New("-", nil)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var defaultEnc encoder.Encoder
	switch *format {
	case "jsonl":
		defaultEnc = encoder.NewJSONLEncoder(defaultRaw)
	default:
		return nil, trace.BadParameter("unsupported format %q", *format)
	}
	return adapter.New(defaultEnc, defaultRaw), nil
}

func run() error {
	ctx := context.TODO()
	app := kingpin.New("ci-normalize", "Normalize test artifacts")
	app.HelpFlag.Short('h')
	format := app.Flag("format", "Output format").Short('f').Default("jsonl").Enum("jsonl")
	metaOuts := app.Flag(
		"meta",
		"Metadata output ('-' for stdout, /dev/null to ignore)",
	).Short('o').Default("-").Strings()

	// metadata command
	metaCmd := app.Command("meta", "Emit metadata")

	// JUnit command
	junitCmd := app.Command("junit", "Normalize JUnit test results")
	suiteOuts := junitCmd.Flag("suites", "Testsuite output(s) ('-' for stdout, /dev/null to ignore)").Default("-").Strings()
	testOuts := junitCmd.Flag("tests", "Testcase output(s) ('-' for stdout, /dev/null to ignore)").Default("-").Strings()
	junitFiles := junitCmd.Arg("files", "JUnit XML result files").Required().ExistingFiles()
	metaFile := junitCmd.Flag("from-meta", "Optionally provide existing metadata").ExistingFile()

	cmd, err := app.Parse(os.Args[1:])
	if err != nil {
		return trace.Wrap(err, "failed to parse command line arguments")

	}

	metadata, err := meta.New(metaFile)
	if err != nil {
		return trace.Wrap(err, "reading metadata")
	}

	// Setup output writers
	opts := []dispatch.Option{}
	add := func(proto any, paths []string) error {
		for _, path := range paths {
			raw, err := writer.New(path, metadata)
			if err != nil {
				return err
			}
			var enc encoder.Encoder
			switch *format {
			case "jsonl":
				enc = encoder.NewJSONLEncoder(raw)
			default:
				return trace.BadParameter("unsupported format %q", *format)
			}
			rw := adapter.New(enc, raw)
			opts = append(opts, dispatch.WithWriter(proto, rw))
		}
		return nil
	}

	if err := add(&record.Suite{}, *suiteOuts); err != nil {
		return trace.Wrap(err)
	}
	if err := add(&record.Testcase{}, *testOuts); err != nil {
		return trace.Wrap(err)
	}
	if err := add(&record.Meta{}, *metaOuts); err != nil {
		return trace.Wrap(err)
	}
	defWriter, err := makeDefaultWriter(format)
	if err != nil {
		return trace.Wrap(err)
	}
	opts = append(opts, dispatch.WithDefaultWriter(defWriter))

	dispatcher, err := dispatch.New(opts...)
	if err != nil {
		return trace.Wrap(err)
	}
	defer dispatcher.Close()

	var producers []input.Producer
	popts := []input.Option{input.WithMeta(metadata)}
	producers = append(producers, input.NewPassthroughProducer(metadata))

	switch cmd {
	case junitCmd.FullCommand():
		for _, f := range *junitFiles {
			p, err := input.NewJUnitProducer(f, popts...)
			if err != nil {
				return trace.Wrap(err)
			}
			producers = append(producers, p)
		}

	case metaCmd.FullCommand():
	default:
		return trace.NotImplemented("unimplemented command %q", cmd)
	}

	if len(producers) == 0 {
		return trace.BadParameter("nothing to do")
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

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
