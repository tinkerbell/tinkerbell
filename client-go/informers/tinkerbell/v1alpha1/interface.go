// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package v1alpha1

import "github.com/tinkerbell/tinkerbell/client-go/informers/internalinterfaces"

// Interface provides access to v1alpha1 informers.
type Interface interface {
	Hardware() HardwareInformer
}

type version struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a v1alpha1 informer accessor.
func New(factory internalinterfaces.SharedInformerFactory, namespace string, tweak internalinterfaces.TweakListOptionsFunc) Interface {
	return &version{factory: factory, namespace: namespace, tweakListOptions: tweak}
}

// Hardware returns the shared Hardware informer accessor.
func (v *version) Hardware() HardwareInformer {
	return &hardwareInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}
