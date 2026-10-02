package kube

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/template/funcmap"
	"github.com/tinkerbell/tinkerbell/pkg/template/render"
	"k8s.io/apimachinery/pkg/runtime"
)

const templateSkipAnnotation = "tinkerbell.org/render-skip"

// renderHardware returns hw with the templates in its spec rendered against the
// Hardware itself (.hardware) and its resolved references (.references). hw is
// never modified; when it has nothing to render it is returned as is.
func renderHardware(hw *tinkerbell.Hardware, references map[string]any) (*tinkerbell.Hardware, error) {
	skip, err := hardwareRenderSkip(hw)
	if err != nil {
		return nil, fmt.Errorf("render hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	doc, err := runtime.DefaultUnstructuredConverter.ToUnstructured(hw)
	if err != nil {
		return nil, fmt.Errorf("convert hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}
	if !render.HasTemplates(doc, render.WithSkip(skip)) {
		return hw, nil
	}

	// doc is a map, so it is rendered in place.
	if _, err := render.Value(doc, map[string]any{"references": references},
		render.WithSelfKey("hardware"),
		render.WithFuncs(funcmap.New()),
		render.WithSkip(skip),
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

func hardwareRenderSkip(hw *tinkerbell.Hardware) (func(string) bool, error) {
	raw, ok := hw.Annotations[templateSkipAnnotation]
	if !ok {
		return skipRender, nil
	}
	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return nil, fmt.Errorf("annotation %s must be a JSON array of string paths: %w", templateSkipAnnotation, err)
	}
	if paths == nil {
		return nil, fmt.Errorf("annotation %s must be a JSON array of string paths, not null", templateSkipAnnotation)
	}
	skipped := make(map[string]bool, len(paths))
	for _, path := range paths {
		if !strings.HasPrefix(path, "spec.") || path == "spec." {
			return nil, fmt.Errorf("annotation %s path %q must be under spec", templateSkipAnnotation, path)
		}
		skipped[path] = true
	}
	return func(path string) bool { return skipRender(path) || skipped[path] }, nil
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
