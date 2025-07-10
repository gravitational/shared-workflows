package coordination

import (
	"context"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type kubeLeaser struct {
	client         kubernetes.Interface
	namespace      string
	holderIdentity string
	log            *slog.Logger
}

func newKubeLeaser(client kubernetes.Interface, namespace, holderIdentity string, log *slog.Logger) *kubeLeaser {
	return &kubeLeaser{
		client:         client,
		namespace:      namespace,
		holderIdentity: holderIdentity,
		log:            log,
	}
}

// AcquireLease acquires a lease in Kubernetes using the leader election library.
func (k *kubeLeaser) AcquireLease(ctx context.Context, leaseName string, duration time.Duration) error {
	acq := make(chan bool)

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{Name: leaseName, Namespace: k.namespace},
		Client:    k.client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: k.holderIdentity,
		},
	}

	lec, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   duration,
		RenewDeadline:   duration / 2,
		RetryPeriod:     time.Second * 2,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			// This function is called when the current process becomes the leader
			// If the lease has already been acquired by another process, this function will not be called
			OnStartedLeading: func(ctx context.Context) {
				k.log.Info("Acquired lease", "lease", leaseName)
				acq <- true
			},
			// This function is always called when the RunOrDie function returns
			// In our case, this will be called when the context is cancelled or if the lease could not be acquired
			OnStoppedLeading: func() {
				k.log.Info("Lost lease", "lease", leaseName)
			},
		},
	})
	if err != nil {
		return err
	}

	go func() {
		lec.Run(ctx)
	}()

	if !<-acq { // Assume that if we could not acquire the lease, it was already acquired by another process.
		// This may not be the case if the kube client is having CRUD issues, for now we will rely on logs.
		return ErrAlreadyLeased
	}
	close(acq)

	return nil
}
