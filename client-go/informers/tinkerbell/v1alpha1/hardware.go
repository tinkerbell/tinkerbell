// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package v1alpha1 provides shared informers for tinkerbell.org/v1alpha1.
package v1alpha1

import (
	"context"
	"time"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/client-go/informers/internalinterfaces"
	"github.com/tinkerbell/tinkerbell/client-go/kubernetes"
	listersv1alpha1 "github.com/tinkerbell/tinkerbell/client-go/listers/tinkerbell/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

// HardwareInformer provides a shared informer and typed lister.
type HardwareInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() listersv1alpha1.HardwareLister
}

type hardwareInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewHardwareInformer constructs an independent Hardware informer.
func NewHardwareInformer(client kubernetes.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredHardwareInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredHardwareInformer constructs an independent filtered Hardware informer.
func NewFilteredHardwareInformer(client kubernetes.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweak internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	typedClient := client.TinkerbellV1alpha1().Hardware(namespace)
	return cache.NewSharedIndexInformer(
		cache.ToListWatcherWithWatchListSemantics(&cache.ListWatch{
			ListWithContextFunc: func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
				if tweak != nil {
					tweak(&opts)
				}
				return typedClient.List(ctx, opts)
			},
			WatchFuncWithContext: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
				if tweak != nil {
					tweak(&opts)
				}
				return typedClient.Watch(ctx, opts)
			},
		}, client),
		&tinkv1alpha1.Hardware{},
		resyncPeriod,
		indexers,
	)
}

func (f *hardwareInformer) defaultInformer(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredHardwareInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

// Informer returns the shared Hardware informer.
func (f *hardwareInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&tinkv1alpha1.Hardware{}, f.defaultInformer)
}

// Lister returns a lister backed by the shared informer's cache.
func (f *hardwareInformer) Lister() listersv1alpha1.HardwareLister {
	return listersv1alpha1.NewHardwareLister(f.Informer().GetIndexer())
}
