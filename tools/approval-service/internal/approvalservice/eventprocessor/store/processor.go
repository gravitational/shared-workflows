package store

import (
	"context"
	"errors"

	"github.com/gravitational/teleport/api/types"
)

// ProcessorService is an interface for managing high-level information about event processing.
type ProcessorService interface {
	// StoreProcID stores the processor ID for a given Access Request.
	// This is used to track which processor is appropriate for handling the Access Request.
	StoreProcID(ctx context.Context, req types.AccessRequest, procID string) error
	// GetProcID retrieves the processor ID for a given Access Request.
	// This is used to determine which processor should handle the Access Request.
	GetProcID(ctx context.Context, req types.AccessRequest) (string, error)
}

// processorService implements the ProcessorService interface.
type processorService struct {
}

const procIDLabel = "procid"

func (p *processorService) StoreProcID(ctx context.Context, req types.AccessRequest, procID string) error {
	labels := req.GetStaticLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	if procID == "" {
		return errors.New("processor ID cannot be empty")
	}

	labels["procid"] = procID
	req.SetStaticLabels(labels)
	return nil
}

func (p *processorService) GetProcID(ctx context.Context, req types.AccessRequest) (string, error) {
	labels := req.GetStaticLabels()
	if labels == nil {
		return "", newMissingLabelError(req, []string{procIDLabel})
	}

	procID, ok := labels["procid"]
	if !ok {
		return "", newMissingLabelError(req, []string{procIDLabel})
	}
	return procID, nil
}
