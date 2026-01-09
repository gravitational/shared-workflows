package main

import (
	"context"
	"fmt"
	"os"

	kingpin "github.com/alecthomas/kingpin/v2"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/input"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/multiwriter"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
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
	opts := []multiwriter.Option{}
	if suiteOut != nil {
		opts = append(opts, multiwriter.WithWriter(&record.Suite{}, *suiteOut, *format))
	}
	if testcaseOut != nil {
		opts = append(opts, multiwriter.WithWriter(&record.Testcase{}, *testcaseOut, *format))
	}
	if metaOut != nil {
		opts = append(opts, multiwriter.WithWriter(&record.Meta{}, *metaOut, *format))
	}

	w, err := multiwriter.New(*format, opts...)
	if err != nil {
		return trace.Wrap(err)
	}
	defer w.Close()

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
		return trace.BadParameter("no input files to process")
	}

	// TODO: handle per file errors.
	// TODO: look into parsing files in parallel.
	// For now just process them sequentially.
	for _, p := range producers {
		if err := p.Produce(ctx, w.Write); err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
