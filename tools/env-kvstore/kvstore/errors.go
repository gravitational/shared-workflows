package kvstore

import (
	"fmt"
)

// envStoreError is used to indicate that an environment specific secret or variable
// store could not be retrieved from secrets manager.
type envStoreError struct {
	arn string
}

func (e envStoreError) Error() string {
	return fmt.Sprintf("unable to retrieve environment-specific values from Secrets Manager (ARN: %s)", e.arn)
}
