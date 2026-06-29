// Package account resolves the AWS account ID of the calling credentials.
package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type stsClient interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// CallerAccountID returns the 12-digit AWS account ID of the calling identity.
func CallerAccountID(ctx context.Context, c stsClient) (string, error) {
	out, err := c.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("get caller identity: %w", err)
	}

	if out.Account == nil || *out.Account == "" {
		return "", errors.New("get caller identity: response missing Account")
	}

	return *out.Account, nil
}
