// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package v1alpha1

import (
	"context"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/client-go/kubernetes/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/gentype"
)

// HardwareGetter provides access to Hardware resources.
type HardwareGetter interface {
	Hardware(namespace string) HardwareInterface
}

// HardwareInterface has methods to work with Hardware resources.
type HardwareInterface interface {
	Create(context.Context, *tinkv1alpha1.Hardware, metav1.CreateOptions) (*tinkv1alpha1.Hardware, error)
	Update(context.Context, *tinkv1alpha1.Hardware, metav1.UpdateOptions) (*tinkv1alpha1.Hardware, error)
	UpdateStatus(context.Context, *tinkv1alpha1.Hardware, metav1.UpdateOptions) (*tinkv1alpha1.Hardware, error)
	Delete(context.Context, string, metav1.DeleteOptions) error
	DeleteCollection(context.Context, metav1.DeleteOptions, metav1.ListOptions) error
	Get(context.Context, string, metav1.GetOptions) (*tinkv1alpha1.Hardware, error)
	List(context.Context, metav1.ListOptions) (*tinkv1alpha1.HardwareList, error)
	Watch(context.Context, metav1.ListOptions) (watch.Interface, error)
	Patch(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) (*tinkv1alpha1.Hardware, error)
}

type hardware struct {
	*gentype.ClientWithList[*tinkv1alpha1.Hardware, *tinkv1alpha1.HardwareList]
}

func newHardware(c *TinkerbellV1alpha1Client, namespace string) *hardware {
	return &hardware{gentype.NewClientWithList(
		"hardware",
		c.RESTClient(),
		scheme.ParameterCodec,
		namespace,
		func() *tinkv1alpha1.Hardware { return &tinkv1alpha1.Hardware{} },
		func() *tinkv1alpha1.HardwareList { return &tinkv1alpha1.HardwareList{} },
	)}
}
