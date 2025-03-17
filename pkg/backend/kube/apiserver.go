package kube

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	endpointLivez = "/livez"
	endpointReady = "/readyz"
)

// waitForAPIServer waits for the Kubernetes API server to become healthy.
func (b *Backend) WaitForAPIServer(ctx context.Context, log logr.Logger, maxWaitTime time.Duration, pollInterval time.Duration, client rest.Interface) error {
	if client == nil {
		k, err := kubernetes.NewForConfig(b.ClientConfig)
		if err != nil {
			return err
		}
		client = k.RESTClient()
	}

	// Check health and ready endpoints once before waiting the poll interval.
	errHealth := checkEndpoint(ctx, endpointLivez, "ok", client)
	errReady := checkEndpoint(ctx, endpointReady, "ok", client)
	if errHealth == nil && errReady == nil {
		log.Info("API server is healthy and ready")
		return nil
	}
	log.Info("API server not healthy and ready yet, will retry", "retryInterval", fmt.Sprintf("%v", pollInterval), "reason", errors.Join(errHealth, errReady))

	// Calculate deadline
	deadline := time.Now().Add(maxWaitTime)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			errHealth := checkEndpoint(ctx, endpointLivez, "ok", client)
			errReady := checkEndpoint(ctx, endpointReady, "ok", client)
			if errHealth == nil && errReady == nil {
				log.Info("API server is healthy and ready")
				return nil
			}
			// Log error and continue waiting
			log.Info("API server not healthy and ready yet, will retry", "retryInterval", fmt.Sprintf("%v", pollInterval), "reason", errors.Join(errHealth, errReady))
		}
	}

	return fmt.Errorf("timed out waiting for API server to be healthy and ready after %v", maxWaitTime)
}

func checkEndpoint(ctx context.Context, endpoint, response string, k rest.Interface) error { //nolint:unparam // This is more maintainable as is.
	resp, err := k.Get().AbsPath(endpoint).DoRaw(ctx)
	if err == nil && len(resp) > 0 && strings.EqualFold(string(resp), response) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("API server not ready yet, error: %w", err)
	}
	return fmt.Errorf("API server not ready yet")
}
