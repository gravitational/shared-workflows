package kvstore

// envStoreError is used to indicate that an environment specific secret or variable
// store could not be retrieved from secrets manager.
type envStoreError struct {
	msg string
}

func (e envStoreError) Error() string {
	return e.msg
}
