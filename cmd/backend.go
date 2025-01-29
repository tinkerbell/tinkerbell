package cmd

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/backend/file"
	"github.com/tinkerbell/tinkerbell/backend/kube"
	"github.com/tinkerbell/tinkerbell/backend/noop"
)

func NewKubeBackend(ctx context.Context, kubeconfig, apiurl, namespace string) (*kube.Backend, error) {
	kb, err := kube.NewBackend(kube.Backend{
		ConfigFilePath: kubeconfig,
		APIURL:         apiurl,
		Namespace:      namespace,
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

func NewFileBackend(ctx context.Context, logger logr.Logger, filepath string) (*file.Watcher, error) {
	f, err := file.NewWatcher(logger, filepath)
	if err != nil {
		return nil, err
	}

	go f.Start(ctx)

	return f, nil
}

func NewNoopBackend() *noop.Backend {
	return &noop.Backend{}
}
