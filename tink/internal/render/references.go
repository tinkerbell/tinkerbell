package render

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ReferenceRules holds the allow/deny-list rules used to decide whether a
// Hardware.Spec.References entry may be resolved and exposed to a Template.
type ReferenceRules struct {
	Allowlist []string
	Denylist  []string
}

// DefaultDenylist returns a deny-list that denies every Hardware.Spec.References entry,
// the effective policy whenever no explicit deny-list rule is configured.
func DefaultDenylist() []string {
	return []string{`{"reference": {"name": [{"wildcard": "*"}]}}`}
}

// DynamicReader reads a single arbitrary-GVR object as an unstructured map, used to
// resolve Hardware.Spec.References.
type DynamicReader interface {
	DynamicRead(ctx context.Context, gvr schema.GroupVersionResource, name, namespace string) (map[string]interface{}, error)
}

// ResolveReferences evaluates hardware.Spec.References against rules and reads each
// allowed reference via dc, returning a map keyed by reference name suitable for
// Input.References. References that are denied, fail evaluation, or fail to read are
// skipped rather than aborting the others; all such failures are joined into the
// returned error so the caller can log or surface them, but a partial reference map is
// still usable for rendering.
func ResolveReferences(ctx context.Context, dc DynamicReader, rules ReferenceRules, hardware tinkerbell.Hardware) (map[string]interface{}, error) {
	references := make(map[string]interface{})
	if len(hardware.Spec.References) == 0 {
		// Skip building rule matchers for the common case of no References at all.
		return references, nil
	}
	var errs error

	// The rules are the same for every entry, so build each matcher once per call.
	denyMatcher, denyErr := newRuleMatcher(rules.Denylist)
	allowMatcher, allowErr := newRuleMatcher(rules.Allowlist)

	// Sorted so the joined error, which ends up in the Workflow's status condition, is
	// stable across reconciles.
	for _, refName := range slices.Sorted(maps.Keys(hardware.Spec.References)) {
		rf := hardware.Spec.References[refName]
		ed := evaluationData{
			Source: source{
				Name:      hardware.Name,
				Namespace: hardware.Namespace,
			},
			Reference: rf,
		}
		if denyErr != nil {
			errs = errors.Join(errs, fmt.Errorf("evaluating denylist for reference %q: %w", refName, denyErr))
			continue
		}
		denied, _, err := evaluate(denyMatcher, ed)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("evaluating denylist for reference %q: %w", refName, err))
			continue
		}
		if allowErr != nil {
			errs = errors.Join(errs, fmt.Errorf("evaluating allowlist for reference %q: %w", refName, allowErr))
			continue
		}
		allowed, _, err := evaluate(allowMatcher, ed)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("evaluating allowlist for reference %q: %w", refName, err))
			continue
		}
		if denied && !allowed {
			errs = errors.Join(errs, fmt.Errorf("reference %q denied", refName))
			continue
		}
		gvr := schema.GroupVersionResource{Group: rf.Group, Version: rf.Version, Resource: rf.Resource}
		if v, err := dc.DynamicRead(ctx, gvr, rf.Name, rf.Namespace); err == nil || v != nil {
			references[refName] = v
		} else {
			errs = errors.Join(errs, fmt.Errorf("reading reference %q: %w", refName, err))
		}
	}
	return references, errs
}
