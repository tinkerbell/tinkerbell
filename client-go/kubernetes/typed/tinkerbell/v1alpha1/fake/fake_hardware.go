// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package fake

import (
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	typedv1alpha1 "github.com/tinkerbell/tinkerbell/client-go/kubernetes/typed/tinkerbell/v1alpha1"
	"k8s.io/client-go/gentype"
)

type fakeHardware struct {
	*gentype.FakeClientWithList[*tinkv1alpha1.Hardware, *tinkv1alpha1.HardwareList]
}

func newFakeHardware(fake *TinkerbellV1alpha1, namespace string) typedv1alpha1.HardwareInterface {
	return &fakeHardware{gentype.NewFakeClientWithList(
		fake.Fake,
		namespace,
		tinkv1alpha1.GroupVersion.WithResource("hardware"),
		tinkv1alpha1.GroupVersion.WithKind("Hardware"),
		func() *tinkv1alpha1.Hardware { return &tinkv1alpha1.Hardware{} },
		func() *tinkv1alpha1.HardwareList { return &tinkv1alpha1.HardwareList{} },
		func(dst, src *tinkv1alpha1.HardwareList) { dst.ListMeta = src.ListMeta },
		func(list *tinkv1alpha1.HardwareList) []*tinkv1alpha1.Hardware {
			return gentype.ToPointerSlice(list.Items)
		},
		func(list *tinkv1alpha1.HardwareList, items []*tinkv1alpha1.Hardware) {
			list.Items = gentype.FromPointerSlice(items)
		},
	)}
}
