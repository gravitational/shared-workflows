package main

import (
	"context"
	"fmt"
	"os"

	kingpin "github.com/alecthomas/kingpin/v2"
	"golang.org/x/sync/errgroup"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/dispatch"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/encoder"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/input"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record/adapter"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/writer"
	"github.com/gravitational/trace"
)

func run() error {
	ctx := context.TODO()
	app := kingpin.New("ci-normalize", "Normalize test artifacts")
	app.HelpFlag.Short('h')
	format := app.Flag("format", "Output format").Short('f').Default("jsonl").Enum("jsonl")

	// metadata command
	metaCmd := app.Command("meta", "Emit metadata")
	metaOut := metaCmd.Flag("out", "Testcase output file ('-' for stdout, /dev/null to ignore)").Short('o').Default("-").String()
	// Only github is supported today.

	// JUnit command
	junitCmd := app.Command("junit", "Normalize JUnit test results")
	suiteOut := junitCmd.Flag("suite", "Suite output file ('-' for stdout, /dev/null to ignore)").String()
	testcaseOut := junitCmd.Flag("testcase", "Testcase output file ('-' for stdout, /dev/null to ignore)").String()
	junitFiles := junitCmd.Arg("files", "JUnit XML result files").Required().ExistingFiles()
	metaFile := junitCmd.Flag("meta", "json metadata file").Required().ExistingFile()

	cmd, err := app.Parse(os.Args[1:])
	if err != nil {
		return trace.Wrap(err, "failed to parse command line arguments")

	}

	// Setup output writers
	opts := []dispatch.Option{}

	add := func(proto any, path *string) error {
		if path == nil || *path == "" {
			return nil
		}

		raw, err := writer.New(*path) // io.WriteCloser
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
		return nil
	}

	if err := add(&record.Suite{}, suiteOut); err != nil {
		return trace.Wrap(err)
	}
	if err := add(&record.Testcase{}, testcaseOut); err != nil {
		return trace.Wrap(err)
	}
	if err := add(&record.Meta{}, metaOut); err != nil {
		return trace.Wrap(err)
	}

	defaultRaw, err := writer.New("-")
	if err != nil {
		return trace.Wrap(err)
	}
	var defaultEnc encoder.Encoder
	switch *format {
	case "jsonl":
		defaultEnc = encoder.NewJSONLEncoder(defaultRaw)
	default:
		return trace.BadParameter("unsupported format %q", *format)
	}
	opts = append(opts, dispatch.WithDefaultWriter(adapter.New(defaultEnc, defaultRaw)))

	dispatcher, err := dispatch.New(opts...)
	if err != nil {
		return trace.Wrap(err)
	}
	defer dispatcher.Close()

	var producers []input.Producer
	popts := []input.Option{input.WithMetaFile(*metaFile)}

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
		producers = append(producers, input.NewGithubMetaProducer())
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
