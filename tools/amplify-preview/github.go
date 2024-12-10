package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gravitational/shared-workflows/libs/github"
)

const (
	prRefNameSuffix = "/merge"
)

func postPreviewURL(ctx context.Context, commentBody string) error {
	refName := os.Getenv("GITHUB_REF_NAME")
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
	}

	comment, err := gh.FindCommentByTraits(ctx, currentPR, targetComment)
	if err != nil && !errors.Is(err, github.ErrCommentNotFound) {
		return fmt.Errorf("something went wrong while searching for comment: %w", err)
	}

	if comment == nil {
		return gh.CreateComment(ctx, currentPR, commentBody)
	} else {
		return gh.UpdateComment(ctx, currentPR, comment.GetID(), commentBody)
	}
}
