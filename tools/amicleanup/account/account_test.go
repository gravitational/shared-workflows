package account

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSTS struct {
	out *sts.GetCallerIdentityOutput
	err error
}

func (f *fakeSTS) GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, _ ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

func TestCallerAccountID_Success(t *testing.T) {
	f := &fakeSTS{out: &sts.GetCallerIdentityOutput{Account: ptr.String("123456789012")}}
	got, err := CallerAccountID(t.Context(), f)
	require.NoError(t, err)
	assert.Equal(t, "123456789012", got)
}

func TestCallerAccountID_NilAccount(t *testing.T) {
	f := &fakeSTS{out: &sts.GetCallerIdentityOutput{Account: nil}}
	_, err := CallerAccountID(t.Context(), f)
	require.Error(t, err)
}

func TestCallerAccountID_APIError(t *testing.T) {
	want := errors.New("sts boom")
	f := &fakeSTS{err: want}
	_, err := CallerAccountID(t.Context(), f)
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}
