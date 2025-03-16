package kube

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func (b *Backend) RestInterface() rest.Interface {
	k, err := kubernetes.NewForConfig(b.ClientConfig)
	if err != nil {
		panic(err)
	}
	return k.RESTClient()
}

// waitForAPIServer waits for the Kubernetes API server to become healthy.
func (b *Backend) WaitForAPIServer(ctx context.Context, log logr.Logger, maxWaitTime time.Duration, pollInterval time.Duration) error {
	k, err := kubernetes.NewForConfig(b.ClientConfig)
	if err != nil {
		return err
	}
	// Create clientset
	clientset := kubernetes.New(k.RESTClient())

	// Calculate deadline
	deadline := time.Now().Add(maxWaitTime)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Check health
			resp, err := clientset.Discovery().RESTClient().Get().AbsPath("/livez").Do(ctx).Raw()

			if err == nil && len(resp) > 0 {
				log.Info("API server is healthy")
				return nil // API server is healthy
			}

			// Log error and continue waiting
			log.Info("API server not healthy yet, will retry", "retryInterval", fmt.Sprintf("%v", pollInterval), "reason", err)

			// Sleep before next check
			time.Sleep(pollInterval)
		}
	}

	return fmt.Errorf("timed out waiting for API server health after %v", maxWaitTime)
}
