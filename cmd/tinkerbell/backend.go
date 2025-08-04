package main

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/backend/file"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/pkg/backend/noop"
)

const (
	defaultQPS   = 100
	defaultBurst = 100
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
	kb, err := kube.NewBackend(kube.Backend{
		ConfigFilePath: kubeconfig,
		APIURL:         apiurl,
		Namespace:      namespace,
		Indexes:        indexes,
		QPS:            defaultQPS,   // Default QPS value. A negative value disables client-side ratelimiting.
		Burst:          defaultBurst, // Default burst value.
	})
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(kb)
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
