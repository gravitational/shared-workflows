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
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gravitational/shared-workflows/bot/internal/bot"
	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"

	"github.com/gravitational/trace"
)

const (
	assignWorkflow   = "assign"
	checkWorkflow    = "check"
	dismissWorkflow  = "dismiss"
	labelWorkflow    = "label"
	backportWorkflow = "backport"
)

var allWorkflows = []string{assignWorkflow, checkWorkflow, dismissWorkflow, labelWorkflow, backportWorkflow}

func main() {
	flags, err := parseFlags(os.Args[1:])
	switch {
	case err == flag.ErrHelp:
		os.Exit(0)
	case err != nil:
		log.Fatalf("Failed to parse flags: %v.", err)
	}

	// Cancel run if it takes longer than 5 minutes.
	//
	// To re-run a job go to the Actions tab in the Github repo, go to the run
	// that failed, and click the "Re-run all jobs" button in the top right corner.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	b, err := createBot(ctx, flags)
	if err != nil {
		log.Fatalf("Failed to create bot: %v.", err)
	}

	log.Printf("Running %v.", flags.workflow)

	switch flags.workflow {
	case assignWorkflow:
		err = b.Assign(ctx)
	case checkWorkflow:
		err = b.Check(ctx)
	case dismissWorkflow:
		err = b.Dismiss(ctx)
	case labelWorkflow:
		err = b.Label(ctx)
	case backportWorkflow:
		if flags.local {
			err = b.BackportLocal(ctx, flags.branch)
		} else {
			err = b.Backport(ctx)
		}
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
	// token is the Github auth token.
	token string
	// reviewers is the code reviewers map.
	reviewers string
	// local is whether workflow runs locally or in Github Actions context.
	local bool
	// org is the Github organization for the local mode.
	org string
	// repo is the Github repository for the local mode.
	repo string
	// prNumber is the Github pull request number for the local mode.
	prNumber int
	// branch is the Github backport branch name for the local mode.
	branch string
}

func parseFlags(args []string) (flags, error) {
	var flag flag.FlagSet
	var (
		workflow  = flag.String("workflow", "", fmt.Sprintf("specific workflow to run %v", allWorkflows))
		token     = flag.String("token", "", "GitHub authentication token")
		reviewers = flag.String("reviewers", "", "reviewer assignments")
		local     = flag.Bool("local", false, "local workflow dry run")
		org       = flag.String("org", "", "Github organization (local mode only)")
		repo      = flag.String("repo", "", "Github repository (local mode only)")
		prNumber  = flag.Int("pr", 0, "Github pull request number (local mode only)")
		branch    = flag.String("branch", "", "Github backport branch name (local mode only)")
	)

	if err := flag.Parse(args); err != nil {
		// Don't wrap this error so that we can exit appropriately in the caller.
		return flags{}, err
	}

	if *workflow == "" {
		return flags{}, trace.BadParameter("workflow missing")
	}
	if *token == "" {
		return flags{}, trace.BadParameter("token missing")
	}

	workflowNeedsReviewers := (*workflow == assignWorkflow || *workflow == checkWorkflow) && !*local
	if workflowNeedsReviewers && *reviewers == "" {
		return flags{}, trace.BadParameter("reviewers required for assign and check")
	}

	var data []byte
	if workflowNeedsReviewers {
		var err error
		data, err = base64.StdEncoding.DecodeString(*reviewers)
		if err != nil {
			return flags{}, trace.Wrap(err)
		}
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
	var reviewer *review.Assignments
	if flags.reviewers != "" {
		reviewer, err = review.FromString(flags.reviewers)
		if err != nil {
			return nil, trace.Wrap(err)
		}
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
// instead of inside Github Actions environment.
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
