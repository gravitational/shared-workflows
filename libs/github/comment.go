package github

import (
	"context"
	"errors"
	"strings"

	"github.com/google/go-github/v63/github"
)

var (
	ErrCommentNotFound = errors.New("comment not found")
)

type IssueIdentifier struct {
	Owner  string
	Repo   string
	Number int
}

// every trait is treated as AND
type CommentTraits struct {
	BodyContains *string
	UserLogin    *string
}

func (c *Client) FindCommentByTraits(ctx context.Context, issue IssueIdentifier, targetComment CommentTraits) (*github.IssueComment, error) {
	comments, _, err := c.client.Issues.ListComments(ctx, issue.Owner, issue.Repo, issue.Number, nil)
	if err != nil {
		return nil, err
	}

	for _, c := range comments {
		matcher := true
		if targetComment.UserLogin != nil {
			matcher = matcher &&
				c.User != nil && c.User.Login != nil &&
				*c.User.Login == *targetComment.UserLogin
		}

		if targetComment.BodyContains != nil {
			matcher = matcher &&
				c.Body != nil &&
				strings.Contains(*c.Body, *targetComment.BodyContains)
		}

		if matcher {
			return c, nil
		}
	}

	return nil, ErrCommentNotFound
}

func (c *Client) CreateComment(ctx context.Context, issue IssueIdentifier, commentBody string) error {
	_, _, err := c.client.Issues.CreateComment(ctx, issue.Owner, issue.Repo, issue.Number, &github.IssueComment{
		Body: &commentBody,
	})

	return err
}

func (c *Client) UpdateComment(ctx context.Context, issue IssueIdentifier, commentId int64, commentBody string) error {
	_, _, err := c.client.Issues.EditComment(ctx, issue.Owner, issue.Repo, commentId, &github.IssueComment{
		Body: &commentBody,
	})

	return err
}
