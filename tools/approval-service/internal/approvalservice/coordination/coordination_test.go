package coordination

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func TestKubeLeaser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ns := "test-namespace"
	cs := fake.NewClientset(
		&coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "request-existing-lease",
				Namespace: ns,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       ptr.To("not-this"),
				LeaseDurationSeconds: ptr.To(int32(10000)), // For testing, set a long duration
				AcquireTime: &metav1.MicroTime{
					Time: time.Now().Add(-10 * time.Second),
				},
				RenewTime: &metav1.MicroTime{
					Time: time.Now().Add(-10 * time.Second),
				},
			},
		},
	)

	c, err := NewCoordinator(
		withKubeInitFunc(func() (kubernetes.Interface, string, error) {
			return cs, ns, nil
		}),
		withHolderIdentity("this"),
	)
	require.NoError(t, err)

	tests := map[string]struct {
		leaseName       string
		expectErr       bool
		withContextFunc func(ctx context.Context) (context.Context, context.CancelFunc)
	}{
		"New Lease": {
			leaseName: "new-lease",
			withContextFunc: func(ctx context.Context) (context.Context, context.CancelFunc) {
				return context.WithTimeout(ctx, 1*time.Second)
			},
			expectErr: false,
		},
		"Existing Lease with timeout": {
			leaseName: "existing-lease",
			withContextFunc: func(ctx context.Context) (context.Context, context.CancelFunc) {
				return context.WithTimeout(ctx, 1*time.Second)
			},
			expectErr: true,
		},
		"Cancelled Context": {
			leaseName: "cancelled-lease",
			withContextFunc: func(ctx context.Context) (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(ctx)
				cancel() // Immediately cancel the context
				return ctx, cancel
			},
			expectErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := tt.withContextFunc(ctx)
			defer cancel()
			err := c.LeaseAccessRequest(ctx, tt.leaseName)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
