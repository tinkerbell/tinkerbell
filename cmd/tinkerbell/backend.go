package main

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/backend/file"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/pkg/backend/noop"
)

func newKubeBackend(ctx context.Context, kubeconfig, apiurl, namespace string, indexes map[kube.IndexType]kube.Index) (*kube.Backend, error) {
	kb, err := kube.NewBackend(kube.Backend{
		ConfigFilePath: kubeconfig,
		APIURL:         apiurl,
		Namespace:      namespace,
		Indexes:        indexes,
	})
	if err != nil {
		return nil, err
	}

	go func() {
		err = kb.Start(ctx)
		if err != nil {
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
