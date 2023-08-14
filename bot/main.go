/*
Copyright 2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/base64"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gravitational/shared-workflows/bot/internal/bot"
	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"

	"github.com/gravitational/trace"
)

func main() {
	flags, err := parseFlags()
	if err != nil {
		log.Fatalf("Failed to parse flags: %v.", err)
	}

	// Cancel run if it takes longer than 5 minutes.
	//
	// To re-run a job go to the Actions tab in the GitHub repo, go to the run
	// that failed, and click the "Re-run all jobs" button in the top right corner.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	b, err := createBot(ctx, flags)
	if err != nil {
		log.Fatalf("Failed to create bot: %v.", err)
	}

	log.Printf("Running %v.", flags.workflow)

	switch flags.workflow {
	case "assign":
		err = b.Assign(ctx)
	case "check":
		err = b.Check(ctx)
	case "dismiss":
		err = b.Dismiss(ctx)
	case "label":
		err = b.Label(ctx)
	case "backport":
		if flags.local {
			err = b.BackportLocal(ctx, flags.branch)
		} else {
			err = b.Backport(ctx)
		}
	case "verify":
		err = b.Verify(ctx)
	case "exclude-flakes":
		err = b.ExcludeFlakes(ctx)
	case "bloat":
		if flags.baseStats != "" {
			err = b.BloatCheck(ctx, flags.baseStats, flags.current, flags.artifacts, os.Stdout)
		} else {
			err = b.BloatCheckDirectories(ctx, flags.base, flags.current, flags.artifacts, os.Stdout)
		}
	case "save-stats":
		err = b.SaveBaseStats(ctx, flags.base, flags.artifacts, os.Stdout)
	default:
		err = trace.BadParameter("unknown workflow: %v", flags.workflow)
	}
	if err != nil {
		log.Fatalf("Workflow %v failed: %v.", flags.workflow, err)
	}

	log.Printf("Workflow %v complete.", flags.workflow)
}

type flags struct {
	// workflow is the name of workflow to run.
	workflow string
	// token is the GitHub auth token.
	token string
	// reviewers is the code reviewers map.
	reviewers string
	// local is whether workflow runs locally or in GitHub Actions context.
	local bool
	// org is the GitHub organization for the local mode.
	org string
	// repo is the GitHub repository for the local mode.
	repo string
	// prNumber is the GitHub pull request number for the local mode.
	prNumber int
	// branch is the GitHub backport branch name for the local mode.
	branch string
	// artifacts are the binaries to analyze for bloat.
	artifacts []string
	// base is the absolute path to a directory containing the base artifacts to bloat check.
	base string
	// baseStats is the absolute path to a file containing the base build artifacts sizes to bloat check.
	baseStats string
	// current is the absolute path to a directory containing the current artifacts to bloat check.
	current string
}

func parseFlags() (flags, error) {
	var (
		workflow  = flag.String("workflow", "", "specific workflow to run [assign, check, dismiss, backport]")
		token     = flag.String("token", "", "GitHub authentication token")
		reviewers = flag.String("reviewers", "", "reviewer assignments")
		local     = flag.Bool("local", false, "local workflow dry run")
		org       = flag.String("org", "", "GitHub organization (local mode only)")
		repo      = flag.String("repo", "", "GitHub repository (local mode only)")
		prNumber  = flag.Int("pr", 0, "GitHub pull request number (local mode only)")
		branch    = flag.String("branch", "", "GitHub backport branch name (local mode only)")
		base      = flag.String("base", "", "an absolute path to a base directory containing artifacts to be checked for bloat")
		baseStats = flag.String("base-stats", "", "an absolute path to a file containing stats for the base build")
		current   = flag.String("current", "", "an absolute path to a branch directory containing artifacts to be checked for bloat")
		artifacts = flag.String("artifacts", "", "a comma separated list of compile artifacts to analyze for bloat")
	)
	flag.Parse()

	if *workflow == "" {
		return flags{}, trace.BadParameter("workflow missing")
	}
	if *token == "" {
		return flags{}, trace.BadParameter("token missing")
	}
	if *reviewers == "" && !*local {
		return flags{}, trace.BadParameter("reviewers required for assign and check")
	}

	data, err := base64.StdEncoding.DecodeString(*reviewers)
	if err != nil {
		return flags{}, trace.Wrap(err)
	}

	stats, err := base64.StdEncoding.DecodeString(*baseStats)
	if err != nil {
		return flags{}, trace.Wrap(err)
	}

	return flags{
		workflow:  *workflow,
		token:     *token,
		reviewers: string(data),
		local:     *local,
		org:       *org,
		repo:      *repo,
		prNumber:  *prNumber,
		branch:    *branch,
		artifacts: strings.Split(*artifacts, ","),
		base:      *base,
		baseStats: string(stats),
		current:   *current,
	}, nil
}

func createBot(ctx context.Context, flags flags) (*bot.Bot, error) {
	if flags.local {
		return createBotLocal(ctx, flags)
	}
	gh, err := github.New(ctx, flags.token)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	environment, err := env.New()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	reviewer, err := review.FromString(flags.reviewers)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	b, err := bot.New(&bot.Config{
		GitHub:      gh,
		Environment: environment,
		Review:      reviewer,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return b, nil
}

// createBotLocal creates a local instance of the bot that can be run locally
// instead of inside GitHub Actions environment.
func createBotLocal(ctx context.Context, flags flags) (*bot.Bot, error) {
	gh, err := github.New(ctx, flags.token)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return bot.New(&bot.Config{
		GitHub: gh,
		Environment: &env.Environment{
			Organization: flags.org,
			Repository:   flags.repo,
			Number:       flags.prNumber,
		},
	})
}
