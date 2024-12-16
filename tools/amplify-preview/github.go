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
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/gravitational/shared-workflows/libs/github"
)

const (
	prRefNameSuffix = "/merge"
)

func postPreviewURL(ctx context.Context, commentBody string) error {
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
		log.Fatalf("Failed to extract PR ID from GITHUB_REF_NAME=%s: %s", refName, err)
	}

	targetComment := github.CommentTraits{
		BodyContains: aws.String(amplifyMarkdownHeader),
	}

	currentPR := github.IssueIdentifier{
		Number: prID,
		Owner:  strings.Split(githubRepository, "/")[0],
		Repo:   strings.Split(githubRepository, "/")[1],
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
