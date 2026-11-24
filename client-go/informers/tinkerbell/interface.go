// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package tinkerbell provides group-level informer accessors.
package tinkerbell

import (
	"github.com/tinkerbell/tinkerbell/client-go/informers/internalinterfaces"
	v1alpha1 "github.com/tinkerbell/tinkerbell/client-go/informers/tinkerbell/v1alpha1"
)

// Interface provides access to each supported version.
type Interface interface {
	V1alpha1() v1alpha1.Interface
}

type group struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a group informer accessor.
func New(factory internalinterfaces.SharedInformerFactory, namespace string, tweak internalinterfaces.TweakListOptionsFunc) Interface {
	return &group{factory: factory, namespace: namespace, tweakListOptions: tweak}
}

// V1alpha1 returns the v1alpha1 informer accessor.
func (g *group) V1alpha1() v1alpha1.Interface {
	return v1alpha1.New(g.factory, g.namespace, g.tweakListOptions)
}
