package github

import (
	"context"
	"errors"
	"strings"

	"github.com/google/go-github/v69/github"
)

var (
	ErrCommentNotFound = errors.New("comment not found")
)

// IssueIdentifier represents an issue or PR on GitHub
type IssueIdentifier struct {
	Owner  string
	Repo   string
	Number int
}

// CommentTraits defines optional traits to filter comments.
// Every trait (if non-empty-string) is matched with an "AND" operator
type CommentTraits struct {
	BodyContains string
	UserLogin    string
}

// FindCommentByTraits searches for a comment in an PR or issue based on specified traits
func (c *Client) FindCommentByTraits(ctx context.Context, issue IssueIdentifier, targetComment CommentTraits) (*github.IssueComment, error) {
	comments, _, err := c.client.Issues.ListComments(ctx, issue.Owner, issue.Repo, issue.Number, nil)
	if err != nil {
		return nil, err
	}

	for _, c := range comments {
		matcher := true
		if targetComment.UserLogin != "" {
			matcher = matcher && c.User.GetLogin() == targetComment.UserLogin
		}

		if targetComment.BodyContains != "" {
			matcher = matcher && strings.Contains(c.GetBody(), targetComment.BodyContains)
		}

		if matcher {
			return c, nil
		}
	}

	return nil, ErrCommentNotFound
}

// CreateComment creates a new comment on an issue or PR
func (c *Client) CreateComment(ctx context.Context, issue IssueIdentifier, commentBody string) error {
	_, _, err := c.client.Issues.CreateComment(ctx, issue.Owner, issue.Repo, issue.Number, &github.IssueComment{
		Body: &commentBody,
	})

	return err
}

// UpdateComment updates an existing comment on an issue or PR
func (c *Client) UpdateComment(ctx context.Context, issue IssueIdentifier, commentId int64, commentBody string) error {
	_, _, err := c.client.Issues.EditComment(ctx, issue.Owner, issue.Repo, commentId, &github.IssueComment{
		Body: &commentBody,
	})

	return err
}
