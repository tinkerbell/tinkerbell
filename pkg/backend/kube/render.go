package kube

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/template/funcmap"
	"github.com/tinkerbell/tinkerbell/pkg/template/render"
	"k8s.io/apimachinery/pkg/runtime"
)

// renderHardware returns hw with the templates in its spec rendered against the
// Hardware itself (.hardware) and its resolved references (.references). hw is
// never modified; when it has nothing to render it is returned as is.
func renderHardware(hw *tinkerbell.Hardware, references map[string]any) (*tinkerbell.Hardware, error) {
	if !needsRendering(hw) {
		return hw, nil
	}

	doc, err := runtime.DefaultUnstructuredConverter.ToUnstructured(hw)
	if err != nil {
		return nil, fmt.Errorf("convert hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	// doc is a map, so it is rendered in place.
	if _, err := render.Value(doc, map[string]any{"references": references},
		render.WithSelfKey("hardware"),
		render.WithFuncs(funcmap.New()),
		render.WithSkip(skipRender),
	); err != nil {
		return nil, fmt.Errorf("render hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	out := &tinkerbell.Hardware{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(doc, out); err != nil {
		return nil, fmt.Errorf("convert rendered hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}
	hw.Status.DeepCopyInto(&out.Status)

	return out, nil
}

// needsRendering reports whether hw's spec contains a template, so that
// untemplated Hardware is never copied and its references never read.
func needsRendering(hw *tinkerbell.Hardware) bool {
	b, err := json.Marshal(hw.Spec)
	return err != nil || bytes.Contains(b, []byte("{{"))
}

// skipRender reports whether the value at path must never be rendered: anything
// outside spec, the references themselves, and the fields Hardware is looked up
// by, whose indexes are built from the stored object.
func skipRender(path string) bool {
	switch {
	case !strings.HasPrefix(path, "spec."), strings.HasPrefix(path, "spec.references."):
		return true
	case path == "spec.agentID", path == "spec.metadata.instance.id":
		return true
	case strings.HasPrefix(path, "spec.interfaces["):
		return strings.HasSuffix(path, "].dhcp.mac") || strings.HasSuffix(path, "].dhcp.ip.address")
	}
	return false
}
