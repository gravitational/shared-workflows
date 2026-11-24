package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/shared-workflows/tools/amicleanup/actions"
	"github.com/shared-workflows/tools/amicleanup/client"
	"github.com/shared-workflows/tools/amicleanup/ec2iface"
	"github.com/shared-workflows/tools/amicleanup/manager"
	"github.com/shared-workflows/tools/amicleanup/planfile"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type cliFlags struct {
	action            string
	dryRun            bool
	yes               bool
	regionConcurrency int
	planFile          string
	planOnly          bool
}

func parseFlags(args []string) (*cliFlags, error) {
	fs := flag.NewFlagSet("amicleanup", flag.ContinueOnError)
	c := &cliFlags{}
	fs.StringVar(&c.action, "action", "", "lifecycle action: deprecate|make-public|make-private|delete (required)")
	fs.BoolVar(&c.dryRun, "dry-run", true, "if true, log writes instead of performing them")
	fs.BoolVar(&c.yes, "yes", false, "skip TTY confirmation prompt for make-public/delete")
	fs.IntVar(&c.regionConcurrency, "region-concurrency", 8, "max regions processed in parallel")
	fs.StringVar(&c.planFile, "plan-file", "", "path to read/write the plan; enables resumable runs")
	fs.BoolVar(&c.planOnly, "plan-only", false, "enumerate, write the plan to --plan-file, and exit (no apply)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if c.action == "" {
		return nil, errors.New("--action is required")
	}

	switch c.action {
	case actions.NameDeprecate, actions.NameMakePublic, actions.NameMakePrivate, actions.NameDelete:
	default:
		return nil, fmt.Errorf("unknown --action=%q", c.action)
	}

	if c.planOnly && c.planFile == "" {
		return nil, errors.New("--plan-only requires --plan-file")
	}

	if c.regionConcurrency <= 0 {
		return nil, errors.New("--region-concurrency must be positive")
	}

	return c, nil
}

func run() error {
	cli, err := parseFlags(os.Args[1:])
	if err != nil {
		return err
	}

	ctx := context.Background()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	action, err := actions.NewByName(cli.action)
	if err != nil {
		return err
	}

	stsClient := sts.NewFromConfig(awsCfg)
	logOut := os.Stderr

	mgr := manager.New(manager.Config{
		Factory:     buildFactory(awsCfg, cli.dryRun, logOut),
		STS:         stsClient,
		Action:      action,
		DryRun:      cli.dryRun,
		Concurrency: cli.regionConcurrency,
		Log:         logOut,
	})

	store, err := openOrEnumerate(ctx, mgr, cli)
	if err != nil {
		return err
	}
	defer store.Close()

	plan := store.Plan()
	pendingCount := len(store.Pending())
	completedCount := len(plan.Entries) - pendingCount
	fmt.Fprintf(logOut, "%d AMIs across %d entries; %d pending, %d already completed\n",
		uniqueImageCount(plan), len(plan.Entries), pendingCount, completedCount)

	if cli.planOnly {
		fmt.Fprintf(logOut, "wrote plan to %s; --plan-only set, exiting\n", cli.planFile)
		return nil
	}

	if needsConfirmation(cli) && !confirm(logOut) {
		return errors.New("aborted by user")
	}

	results, applyErr := mgr.Apply(ctx, store)
	failed := 0
	for _, r := range results {
		if r.Err != "" {
			failed++
			fmt.Fprintf(logOut, "FAILED %s in %s: %s\n", r.ImageID, r.Region, r.Err)
		}
	}
	fmt.Fprintf(logOut, "applied %d entries (%d failed)\n", len(results), failed)

	if applyErr != nil {
		return applyErr
	}

	if failed > 0 {
		return fmt.Errorf("%d entries failed", failed)
	}

	return nil
}

// openOrEnumerate produces a PlanStore ready for Apply. When --plan-file points
// at an existing file we resume; otherwise we enumerate and write a fresh plan.
// When --plan-file is empty we synthesize an ephemeral path under TMPDIR so the
// in-memory plan still has crash-recovery as a side benefit.
func openOrEnumerate(ctx context.Context, mgr *manager.Manager, cli *cliFlags) (*planfile.PlanStore, error) {
	planPath := cli.planFile
	ephemeral := planPath == ""
	if ephemeral {
		f, err := os.CreateTemp("", "amicleanup-*.json")
		if err != nil {
			return nil, fmt.Errorf("create temp plan: %w", err)
		}

		planPath = f.Name()
		_ = f.Close()
		os.Remove(planPath) // we want the path; planfile.Save will create it.
	}

	if !ephemeral && fileExists(planPath) {
		store, err := planfile.OpenStore(planPath)
		if err != nil {
			return nil, fmt.Errorf("load plan %s: %w", planPath, err)
		}
		if err := mgr.ValidatePlan(ctx, store.Plan()); err != nil {
			return nil, fmt.Errorf("plan %s incompatible with current run: %w", planPath, err)
		}
		fmt.Fprintf(os.Stderr, "resumed plan from %s\n", planPath)
		return store, nil
	}

	plan, regionErrs, err := mgr.Enumerate(ctx)
	if err != nil {
		return nil, err
	}

	for _, re := range regionErrs {
		fmt.Fprintf(os.Stderr, "warning: region %s failed enumeration: %v\n", re.Region, re.Err)
	}

	store, err := planfile.NewStore(planPath, plan)
	if err != nil {
		return nil, err
	}

	if !ephemeral {
		fmt.Fprintf(os.Stderr, "wrote plan to %s\n", planPath)
	}

	return store, nil
}

func buildFactory(awsCfg aws.Config, dryRun bool, log io.Writer) ec2iface.RegionalClientFactory {
	return func(region string) ec2iface.EC2API {
		c := ec2.NewFromConfig(awsCfg, func(o *ec2.Options) { o.Region = region })
		if dryRun {
			return client.NewReadOnly(c, region, log)
		}

		return c
	}
}

func needsConfirmation(cli *cliFlags) bool {
	if cli.dryRun || cli.yes {
		return false
	}
	return cli.action == actions.NameDelete || cli.action == actions.NameMakePublic
}

func confirm(out io.Writer) bool {
	if !isTTY(os.Stdin) {
		fmt.Fprintln(out, "stdin is not a TTY and --yes was not set; refusing to perform a destructive action")
		return false
	}

	fmt.Fprint(out, "Continue? [y/N]: ")
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return false
	}

	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func uniqueImageCount(p *planfile.Plan) int {
	seen := map[string]struct{}{}
	for _, e := range p.Entries {
		seen[e.ImageID] = struct{}{}
	}

	return len(seen)
}
