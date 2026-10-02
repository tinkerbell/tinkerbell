// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package internalinterfaces contains interfaces shared by informer packages.
package internalinterfaces

import (
	"time"

	"github.com/tinkerbell/tinkerbell/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// TweakListOptionsFunc can adjust informer list and watch options.
type TweakListOptionsFunc func(*metav1.ListOptions)

// NewInformerFunc constructs an informer using a client and resync period.
type NewInformerFunc func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer

// SharedInformerFactory is the subset used by generated-style informer accessors.
type SharedInformerFactory interface {
	InformerFor(runtime.Object, NewInformerFunc) cache.SharedIndexInformer
}
