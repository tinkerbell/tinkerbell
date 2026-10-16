// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package v1alpha1 provides typed listers for tinkerbell.org/v1alpha1.
package v1alpha1

import (
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/listers"
	"k8s.io/client-go/tools/cache"
)

// HardwareLister lists Hardware objects across namespaces.
type HardwareLister interface {
	List(labels.Selector) ([]*tinkv1alpha1.Hardware, error)
	Hardware(namespace string) HardwareNamespaceLister
}

type hardwareLister struct {
	listers.ResourceIndexer[*tinkv1alpha1.Hardware]
}

// NewHardwareLister creates a Hardware lister backed by indexer.
func NewHardwareLister(indexer cache.Indexer) HardwareLister {
	return &hardwareLister{listers.New[*tinkv1alpha1.Hardware](indexer, tinkv1alpha1.GroupVersion.WithResource("hardware").GroupResource())}
}

// Hardware returns a namespace-scoped lister.
func (l *hardwareLister) Hardware(namespace string) HardwareNamespaceLister {
	return hardwareNamespaceLister{listers.NewNamespaced[*tinkv1alpha1.Hardware](l.ResourceIndexer, namespace)}
}

// HardwareNamespaceLister lists and gets Hardware in one namespace.
type HardwareNamespaceLister interface {
	List(labels.Selector) ([]*tinkv1alpha1.Hardware, error)
	Get(name string) (*tinkv1alpha1.Hardware, error)
}

type hardwareNamespaceLister struct {
	listers.ResourceIndexer[*tinkv1alpha1.Hardware]
}
