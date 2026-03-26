package kvstore

import (
	"errors"

	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// envStoreNotFoundError is used to indicate that an environment specific secret or variable
// store was expected but not found or is empty.
type envStoreNotFoundError struct {
	msg string
}

func (e envStoreNotFoundError) Error() string {
	return e.msg
}

// isResourceNotFoundException tests whether errors from aws.secretsmanager.GetSecretValue
// indicate the requested secret is missing.
func isResourceNotFoundException(err error) bool {
	var notFoundException *smtypes.ResourceNotFoundException
	return errors.As(err, &notFoundException)
}
