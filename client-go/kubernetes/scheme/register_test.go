// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package scheme

import (
	"fmt"
	"testing"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestHardwareTypesAreRegistered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind   string
		object any
	}{
		{kind: "Hardware", object: &tinkv1alpha1.Hardware{}},
		{kind: "HardwareList", object: &tinkv1alpha1.HardwareList{}},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			t.Parallel()

			gvk := tinkv1alpha1.GroupVersion.WithKind(test.kind)
			object, err := Scheme.New(gvk)
			if err != nil {
				t.Fatalf("Scheme.New(%s): %v", gvk, err)
			}
			if got, want := object, test.object; fmtType(got) != fmtType(want) {
				t.Fatalf("Scheme.New(%s) type = %s, want %s", gvk, fmtType(got), fmtType(want))
			}
		})
	}
}

func fmtType(value any) string {
	return fmt.Sprintf("%T", value)
}
