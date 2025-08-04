package main

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/backend/file"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/pkg/backend/noop"
)

const (
	defaultQPS   float32 = 100
	defaultBurst int     = 100
)

type kubeBackendOpt func(k *kube.Backend)

func WithQPS(qps float32) kubeBackendOpt {
	return func(k *kube.Backend) {
		k.QPS = qps
	}
}

func WithBurst(burst int) kubeBackendOpt {
	return func(k *kube.Backend) {
		k.Burst = burst
	}
}

func newKubeBackend(ctx context.Context, kubeconfig, apiurl, namespace string, indexes map[kube.IndexType]kube.Index, opts ...kubeBackendOpt) (*kube.Backend, error) {
	defaultConfig := kube.Backend{
		ConfigFilePath: kubeconfig,
		APIURL:         apiurl,
		Namespace:      namespace,
		Indexes:        indexes,
		QPS:            defaultQPS,   // Default QPS value. A negative value disables client-side ratelimiting.
		Burst:          defaultBurst, // Default burst value.
	}
	for _, opt := range opts {
		opt(&defaultConfig)
	}
	kb, err := kube.NewBackend(defaultConfig)
	if err != nil {
		return nil, err
	}

	go func() {
		if err := kb.Start(ctx); err != nil {
			panic(err)
		}
	}()

	return kb, nil
}

func newFileBackend(ctx context.Context, logger logr.Logger, filepath string) (*file.Watcher, error) {
	f, err := file.NewWatcher(logger, filepath)
	if err != nil {
		return nil, err
	}

	go f.Start(ctx)

	return f, nil
}

func newNoopBackend() *noop.Backend {
	return &noop.Backend{}
}
