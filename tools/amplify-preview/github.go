/*
Copyright 2024 Gravitational, Inc.

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
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gravitational/shared-workflows/libs/github"
)

func postPreviewURL(ctx context.Context, commentBody string) error {
	const prRefNameSuffix = "/merge"
	refName := os.Getenv("GITHUB_REF_NAME")
	githubRepository := os.Getenv("GITHUB_REPOSITORY")
	if !strings.HasSuffix(refName, prRefNameSuffix) {
		return nil
	}

	gh, err := github.NewClientFromGHAuth(ctx)
	if err != nil {
		return err
	}

	prID, err := strconv.Atoi(strings.TrimSuffix(refName, "/merge"))
	if err != nil {
		return fmt.Errorf("Failed to extract PR ID from GITHUB_REF_NAME=%s: %s", refName, err)
	}

	targetComment := github.CommentTraits{
		BodyContains: amplifyMarkdownHeader,
	}

	githubRepoParts := strings.Split(githubRepository, "/")
	if len(githubRepoParts) < 2 {
		return fmt.Errorf("Couldn't extract repo and owner from %q", githubRepository)
	}
	currentPR := github.IssueIdentifier{
		Number: prID,
		Owner:  githubRepoParts[0],
		Repo:   githubRepoParts[1],
	}

	comment, err := gh.FindCommentByTraits(ctx, currentPR, targetComment)
	if err != nil {
		if errors.Is(err, github.ErrCommentNotFound) {
			return gh.CreateComment(ctx, currentPR, commentBody)
		}
		return fmt.Errorf("something went wrong while searching for comment: %w", err)
	}

	return gh.UpdateComment(ctx, currentPR, comment.GetID(), commentBody)
}
