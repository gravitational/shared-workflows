package approvalservice

import (
	"context"
	"net/http"
)

func (a *ApprovalService) runHealthEndpoint(ctx context.Context) error {
	srv := &http.Server{
		Addr: ":8000",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		}),
	}
	// Start the HTTP server
	go func() {
		a.log.Info("Starting health endpoint", "address", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.log.Error("HTTP server error", "error", err)
		}
	}()

	// Wait for the context to be done
	<-ctx.Done()
	// Shutdown the server gracefully
	if err := srv.Shutdown(ctx); err != nil {
		a.log.Error("HTTP server shutdown error", "error", err)
	}

	return nil
}
