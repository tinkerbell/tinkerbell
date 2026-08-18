// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package fake provides a fake Tinkerbell clientset.
package fake

import (
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/testing"

	clientset "github.com/tinkerbell/tinkerbell/client-go/kubernetes"
	clientscheme "github.com/tinkerbell/tinkerbell/client-go/kubernetes/scheme"
	typedv1alpha1 "github.com/tinkerbell/tinkerbell/client-go/kubernetes/typed/tinkerbell/v1alpha1"
	typedfakev1alpha1 "github.com/tinkerbell/tinkerbell/client-go/kubernetes/typed/tinkerbell/v1alpha1/fake"
)

// Clientset implements kubernetes.Interface using an object tracker.
type Clientset struct {
	testing.Fake
	discovery *fakediscovery.FakeDiscovery
	tracker   testing.ObjectTracker
}

// NewSimpleClientset creates a fake clientset seeded with objects.
func NewSimpleClientset(objects ...runtime.Object) *Clientset {
	tracker := testing.NewObjectTracker(clientscheme.Scheme, clientscheme.Codecs.UniversalDecoder())
	for _, object := range objects {
		var err error
		switch typed := object.(type) {
		case *tinkv1alpha1.Hardware:
			// Hardware's CRD plural is "hardware", while the generic tracker
			// guesses "hardwares". Seed the exact resource used by the client.
			err = tracker.Create(tinkv1alpha1.GroupVersion.WithResource("hardware"), typed, typed.Namespace)
		case *tinkv1alpha1.HardwareList:
			for i := range typed.Items {
				item := &typed.Items[i]
				if err = tracker.Create(tinkv1alpha1.GroupVersion.WithResource("hardware"), item, item.Namespace); err != nil {
					break
				}
			}
		default:
			err = tracker.Add(object)
		}
		if err != nil {
			panic(err)
		}
	}

	cs := &Clientset{tracker: tracker}
	cs.discovery = &fakediscovery.FakeDiscovery{Fake: &cs.Fake}
	cs.AddReactor("*", "*", testing.ObjectReaction(tracker))
	cs.AddWatchReactor("*", func(action testing.Action) (bool, watch.Interface, error) {
		var opts metav1.ListOptions
		if watchAction, ok := action.(interface{ GetListOptions() metav1.ListOptions }); ok {
			opts = watchAction.GetListOptions()
		} else if watchAction, ok := action.(testing.WatchAction); ok {
			restrictions := watchAction.GetWatchRestrictions()
			opts.LabelSelector = restrictions.Labels.String()
			opts.FieldSelector = restrictions.Fields.String()
			opts.ResourceVersion = restrictions.ResourceVersion
		}
		watcher, err := tracker.Watch(action.GetResource(), action.GetNamespace(), opts)
		return true, watcher, err
	})
	return cs
}

// Discovery returns fake discovery.
func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	return c.discovery
}

// Tracker returns the backing object tracker.
func (c *Clientset) Tracker() testing.ObjectTracker {
	return c.tracker
}

// TinkerbellV1alpha1 returns the fake typed client.
func (c *Clientset) TinkerbellV1alpha1() typedv1alpha1.TinkerbellV1alpha1Interface {
	return &typedfakev1alpha1.TinkerbellV1alpha1{Fake: &c.Fake}
}

// IsWatchListSemanticsUnSupported tells reflectors to use list-then-watch.
func (c *Clientset) IsWatchListSemanticsUnSupported() bool {
	return true
}

var (
	_ clientset.Interface = (*Clientset)(nil)
	_ testing.FakeClient  = (*Clientset)(nil)
)
