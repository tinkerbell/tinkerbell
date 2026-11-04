// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package informers

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

// GenericInformer provides an informer and generic lister for a resource.
type GenericInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() cache.GenericLister
}

type genericInformer struct {
	informer cache.SharedIndexInformer
	resource schema.GroupResource
}

func (g *genericInformer) Informer() cache.SharedIndexInformer {
	return g.informer
}

func (g *genericInformer) Lister() cache.GenericLister {
	return cache.NewGenericLister(g.informer.GetIndexer(), g.resource)
}
