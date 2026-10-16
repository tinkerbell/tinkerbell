// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package informers provides a shared informer factory for Tinkerbell APIs.
package informers

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/client-go/informers/internalinterfaces"
	tinkerbellinformers "github.com/tinkerbell/tinkerbell/client-go/informers/tinkerbell"
	"github.com/tinkerbell/tinkerbell/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

// SharedInformerOption configures a shared informer factory.
type SharedInformerOption func(*sharedInformerFactory)

// WithNamespace limits all informers to namespace.
func WithNamespace(namespace string) SharedInformerOption {
	return func(factory *sharedInformerFactory) { factory.namespace = namespace }
}

// WithTweakListOptions configures a function applied to all list/watch options.
func WithTweakListOptions(tweak internalinterfaces.TweakListOptionsFunc) SharedInformerOption {
	return func(factory *sharedInformerFactory) { factory.tweakListOptions = tweak }
}

// SharedInformerFactory provides shared informer lifecycle and accessors.
type SharedInformerFactory interface {
	internalinterfaces.SharedInformerFactory
	ForResource(schema.GroupVersionResource) (GenericInformer, error)
	Start(<-chan struct{})
	Shutdown()
	WaitForCacheSync(<-chan struct{}) map[reflect.Type]bool
	Tinkerbell() tinkerbellinformers.Interface
}

type sharedInformerFactory struct {
	client           kubernetes.Interface
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	defaultResync    time.Duration

	lock             sync.Mutex
	informers        map[reflect.Type]cache.SharedIndexInformer
	startedInformers map[reflect.Type]bool
	shuttingDown     bool
	wg               sync.WaitGroup
}

// NewSharedInformerFactory creates a factory for all namespaces.
func NewSharedInformerFactory(client kubernetes.Interface, defaultResync time.Duration) SharedInformerFactory {
	return NewSharedInformerFactoryWithOptions(client, defaultResync)
}

// NewFilteredSharedInformerFactory creates a namespace-scoped filtered factory.
func NewFilteredSharedInformerFactory(client kubernetes.Interface, defaultResync time.Duration, namespace string, tweak internalinterfaces.TweakListOptionsFunc) SharedInformerFactory {
	return NewSharedInformerFactoryWithOptions(client, defaultResync, WithNamespace(namespace), WithTweakListOptions(tweak))
}

// NewSharedInformerFactoryWithOptions creates a configured factory.
func NewSharedInformerFactoryWithOptions(client kubernetes.Interface, defaultResync time.Duration, options ...SharedInformerOption) SharedInformerFactory {
	factory := &sharedInformerFactory{
		client:           client,
		namespace:        metav1.NamespaceAll,
		defaultResync:    defaultResync,
		informers:        make(map[reflect.Type]cache.SharedIndexInformer),
		startedInformers: make(map[reflect.Type]bool),
	}
	for _, option := range options {
		option(factory)
	}
	return factory
}

// InformerFor returns one shared informer per object type.
func (f *sharedInformerFactory) InformerFor(object runtime.Object, newFunc internalinterfaces.NewInformerFunc) cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()
	typeOf := reflect.TypeOf(object)
	if informer, ok := f.informers[typeOf]; ok {
		return informer
	}
	informer := newFunc(f.client, f.defaultResync)
	f.informers[typeOf] = informer
	return informer
}

// Start starts each requested informer once.
func (f *sharedInformerFactory) Start(stopCh <-chan struct{}) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.shuttingDown {
		return
	}
	for objectType, informer := range f.informers {
		if f.startedInformers[objectType] {
			continue
		}
		f.startedInformers[objectType] = true
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			informer.Run(stopCh)
		}()
	}
}

// Shutdown waits for all started informers to stop.
func (f *sharedInformerFactory) Shutdown() {
	f.lock.Lock()
	f.shuttingDown = true
	f.lock.Unlock()
	f.wg.Wait()
}

// WaitForCacheSync waits for each started informer and returns its sync state.
func (f *sharedInformerFactory) WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool {
	f.lock.Lock()
	started := make(map[reflect.Type]cache.SharedIndexInformer)
	for objectType, informer := range f.informers {
		if f.startedInformers[objectType] {
			started[objectType] = informer
		}
	}
	f.lock.Unlock()

	result := make(map[reflect.Type]bool, len(started))
	for objectType, informer := range started {
		result[objectType] = cache.WaitForCacheSync(stopCh, informer.HasSynced)
	}
	return result
}

// Tinkerbell returns group informer accessors.
func (f *sharedInformerFactory) Tinkerbell() tinkerbellinformers.Interface {
	return tinkerbellinformers.New(f, f.namespace, f.tweakListOptions)
}

// ForResource returns a generic informer for a supported resource.
func (f *sharedInformerFactory) ForResource(resource schema.GroupVersionResource) (GenericInformer, error) {
	switch resource {
	case tinkv1alpha1.GroupVersion.WithResource("hardware"):
		informer := f.Tinkerbell().V1alpha1().Hardware().Informer()
		return &genericInformer{resource: resource.GroupResource(), informer: informer}, nil
	default:
		return nil, fmt.Errorf("no informer found for %v", resource)
	}
}
