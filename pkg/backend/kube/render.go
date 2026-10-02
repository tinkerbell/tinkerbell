package kube

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/template/funcmap"
	"github.com/tinkerbell/tinkerbell/pkg/template/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const templateSkipAnnotation = "tinkerbell.org/render-skip"

//nolint:goconst // Keep the complete built-in skip policy readable in one list.
var skippedHardwarePaths = []string{
	"apiVersion",
	"kind",
	"metadata",
	"status",
	"spec.references",
	"spec.agentID",
	"spec.metadata.instance.id",
	"spec.interfaces[].dhcp.mac",
	"spec.interfaces[].dhcp.ip.address",
}

var protectedHardwarePaths = []string{
	"apiVersion",
	"kind",
	"metadata",
	"status",
	"spec.references",
	"spec.agentID",
	"spec.metadata.instance.id",
	"spec.interfaces[].dhcp.mac",
	"spec.interfaces[].dhcp.ip.address",
}

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
	original := runtime.DeepCopyJSON(doc)

	// doc is a map, so it is rendered in place.
	if _, err := render.Value(doc, map[string]any{"references": runtime.DeepCopyJSON(references)},
		render.WithSelfKey("hardware"),
		render.WithFuncs(funcmap.New()),
		render.WithSkip(skip),
	); err != nil {
		return nil, fmt.Errorf("render hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}
	if err := validateProtectedHardware(original, doc); err != nil {
		return nil, fmt.Errorf("render hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	out := &tinkerbell.Hardware{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(doc, out); err != nil {
		return nil, fmt.Errorf("convert rendered hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}
	return out, nil
}

func validateProtectedHardware(before, after map[string]any) error {
	for _, path := range protectedHardwarePaths {
		if changed := changedProtectedPath(before, after, path); changed != "" {
			return fmt.Errorf("protected field %q changed", changed)
		}
	}
	return nil
}

func changedProtectedPath(before, after map[string]any, path string) string {
	arrayPath, elementPath, iterate := strings.Cut(path, "[].")
	if !iterate {
		if protectedFieldChanged(before, after, strings.Split(path, ".")...) {
			return path
		}
		return ""
	}
	fields := strings.Split(arrayPath, ".")
	oldValue, oldFound, oldErr := unstructured.NestedFieldNoCopy(before, fields...)
	newValue, newFound, newErr := unstructured.NestedFieldNoCopy(after, fields...)
	oldElements, oldSlice := oldValue.([]any)
	newElements, newSlice := newValue.([]any)
	if oldErr != nil || newErr != nil || (oldFound && !oldSlice) || (newFound && !newSlice) {
		return arrayPath
	}
	for index := range max(len(oldElements), len(newElements)) {
		var oldElement, newElement map[string]any
		if index < len(oldElements) {
			var ok bool
			oldElement, ok = oldElements[index].(map[string]any)
			if !ok {
				return fmt.Sprintf("%s[%d]", arrayPath, index)
			}
		}
		if index < len(newElements) {
			var ok bool
			newElement, ok = newElements[index].(map[string]any)
			if !ok {
				return fmt.Sprintf("%s[%d]", arrayPath, index)
			}
		}
		if changed := changedProtectedPath(oldElement, newElement, elementPath); changed != "" {
			return fmt.Sprintf("%s[%d].%s", arrayPath, index, changed)
		}
	}
	return ""
}

func protectedFieldChanged(before, after map[string]any, path ...string) bool {
	oldValue, oldFound, oldErr := unstructured.NestedFieldNoCopy(before, path...)
	newValue, newFound, newErr := unstructured.NestedFieldNoCopy(after, path...)
	return oldErr != nil || newErr != nil || oldFound != newFound || !reflect.DeepEqual(oldValue, newValue)
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

func skipRender(path string) bool {
	if !strings.HasPrefix(path, "spec.") {
		return true
	}
	for _, skipped := range skippedHardwarePaths {
		if matchesSkippedPath(path, skipped) {
			return true
		}
	}
	return false
}

func matchesSkippedPath(path, skipped string) bool {
	arrayPath, elementPath, iterate := strings.Cut(skipped, "[].")
	if !iterate {
		return path == skipped || strings.HasPrefix(path, skipped+".") || strings.HasPrefix(path, skipped+"[")
	}
	remainder, ok := strings.CutPrefix(path, arrayPath+"[")
	if !ok {
		return false
	}
	index, remainder, ok := strings.Cut(remainder, "].")
	if !ok {
		return false
	}
	if _, err := strconv.ParseUint(index, 10, 64); err != nil {
		return false
	}
	return matchesSkippedPath(remainder, elementPath)
}
