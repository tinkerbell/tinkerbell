// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package fake

import (
	"context"
	"testing"
	"time"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktesting "k8s.io/client-go/testing"
)

func TestHardwareCRUDWatchSelectorsAndActions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clientset := NewSimpleClientset(
		&tinkv1alpha1.Hardware{ObjectMeta: metav1.ObjectMeta{Name: "west", Namespace: "rack", Labels: map[string]string{"site": "west"}}},
		&tinkv1alpha1.Hardware{ObjectMeta: metav1.ObjectMeta{Name: "east", Namespace: "rack", Labels: map[string]string{"site": "east"}}},
	)
	if clientset.Discovery() == nil {
		t.Fatal("Discovery() returned nil")
	}
	hardware := clientset.TinkerbellV1alpha1().Hardware("rack")

	list, err := hardware.List(ctx, metav1.ListOptions{LabelSelector: "site=west"})
	if err != nil || len(list.Items) != 1 || list.Items[0].Name != "west" {
		t.Fatalf("filtered List = %#v, %v", list, err)
	}

	watcher, err := hardware.Watch(ctx, metav1.ListOptions{ResourceVersion: list.ResourceVersion})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer watcher.Stop()
	created, err := hardware.Create(ctx, &tinkv1alpha1.Hardware{ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "rack", Labels: map[string]string{"site": "west"}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	select {
	case event := <-watcher.ResultChan():
		if eventObject, ok := event.Object.(*tinkv1alpha1.Hardware); !ok || eventObject.Name != "new" {
			t.Fatalf("watch event object = %#v", event.Object)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for watch event")
	}

	created.Labels["updated"] = "true"
	if _, err := hardware.Update(ctx, created, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := hardware.UpdateStatus(ctx, created, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	actions := clientset.Actions()
	statusAction := actions[len(actions)-1]
	if update, ok := statusAction.(ktesting.UpdateAction); !ok || update.GetSubresource() != "status" {
		t.Fatalf("last action = %#v, want status update", statusAction)
	}
	if err := hardware.Delete(ctx, "new", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := hardware.Get(ctx, "new", metav1.GetOptions{}); err == nil {
		t.Fatal("Get after Delete succeeded")
	}
}
