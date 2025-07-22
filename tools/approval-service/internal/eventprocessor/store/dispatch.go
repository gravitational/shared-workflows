package store

import (
	"context"
	"errors"

	"github.com/gravitational/teleport/api/types"
)

// DispatchStorer is used by the Dispatcher to encode information about which processor should handle a given Access Request.
// Given an Access Request, it should be possible to retrieve information necessary to determine which processor is appropriate for handling the Access Request.
type DispatchStorer interface {
	// StoreProcID stores the processor ID for a given Access Request.
	// This is used to track which processor is appropriate for handling the Access Request.
	StoreProcID(ctx context.Context, req types.AccessRequest, procID string) error
	// GetProcID retrieves the processor ID for a given Access Request.
	// This is used to determine which processor should handle the Access Request.
	GetProcID(ctx context.Context, req types.AccessRequest) (string, error)
}

// dispatchStore implements the DispatchStorer interface.
type dispatchStore struct {
}

const procIDLabel = "procid"

func (p *dispatchStore) StoreProcID(ctx context.Context, req types.AccessRequest, procID string) error {
	if procID == "" {
		return errors.New("processor ID cannot be empty")
	}

	labels := req.GetStaticLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[procIDLabel] = procID
	req.SetStaticLabels(labels)
	return nil
}

func (p *dispatchStore) GetProcID(ctx context.Context, req types.AccessRequest) (string, error) {
	labels := req.GetStaticLabels()

	procID := labels[procIDLabel]
	if procID == "" {
		return "", newMissingLabelError(req, []string{procIDLabel})
	}
	return procID, nil
}
