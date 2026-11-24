// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package informers

import (
	"reflect"
	"testing"
	"time"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	fakeclientset "github.com/tinkerbell/tinkerbell/client-go/kubernetes/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

func TestSharedInformerNamespaceFilterSyncAndLister(t *testing.T) {
	t.Parallel()

	client := fakeclientset.NewSimpleClientset(
		&tinkv1alpha1.Hardware{ObjectMeta: metav1.ObjectMeta{Name: "wanted", Namespace: "rack-a", Labels: map[string]string{"site": "west"}}},
		&tinkv1alpha1.Hardware{ObjectMeta: metav1.ObjectMeta{Name: "filtered", Namespace: "rack-a", Labels: map[string]string{"site": "east"}}},
		&tinkv1alpha1.Hardware{ObjectMeta: metav1.ObjectMeta{Name: "other-namespace", Namespace: "rack-b", Labels: map[string]string{"site": "west"}}},
	)
	factory := NewSharedInformerFactoryWithOptions(client, 0,
		WithNamespace("rack-a"),
		WithTweakListOptions(func(options *metav1.ListOptions) { options.LabelSelector = "site=west" }),
	)
	hardware := factory.Tinkerbell().V1alpha1().Hardware()
	informer := hardware.Informer()

	generic, err := factory.ForResource(tinkv1alpha1.GroupVersion.WithResource("hardware"))
	if err != nil {
		t.Fatalf("ForResource: %v", err)
	}
	if generic.Informer() != informer {
		t.Fatal("ForResource did not return the shared Hardware informer")
	}
	if _, err := factory.ForResource(tinkv1alpha1.GroupVersion.WithResource("unknown")); err == nil {
		t.Fatal("ForResource accepted an unsupported resource")
	}

	stopCh := make(chan struct{})
	factory.Start(stopCh)
	synced := factory.WaitForCacheSync(stopCh)
	if !synced[reflect.TypeOf(&tinkv1alpha1.Hardware{})] {
		t.Fatalf("WaitForCacheSync = %v", synced)
	}

	objects, err := hardware.Lister().List(labels.Everything())
	if err != nil || len(objects) != 1 || objects[0].Name != "wanted" {
		t.Fatalf("Lister.List = %#v, %v", objects, err)
	}
	object, err := hardware.Lister().Hardware("rack-a").Get("wanted")
	if err != nil || object.Name != "wanted" {
		t.Fatalf("namespace lister Get = %#v, %v", object, err)
	}
	indexed, err := informer.GetIndexer().ByIndex(cache.NamespaceIndex, "rack-a")
	if err != nil || len(indexed) != 1 {
		t.Fatalf("namespace index = %#v, %v", indexed, err)
	}

	close(stopCh)
	done := make(chan struct{})
	go func() {
		factory.Shutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return")
	}
}
