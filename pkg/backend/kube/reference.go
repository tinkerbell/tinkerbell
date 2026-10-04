package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"quamina.net/go/quamina"
)

// denyAllReferences is the deny list used when none is configured.
const denyAllReferences = `{"reference": {"name": [{"wildcard": "*"}]}}`

// evaluateData is the data structure used for evaluating rules.
// In Quamina, this is called the "event".
type evaluationData struct {
	// Source is the Object that contains the references.
	Source source `json:"source,omitempty"`
	// Reference is a reference to another Object from the source.
	Reference tinkerbell.Reference `json:"reference,omitempty"`
}

// source is the Object that contains the references.
type source struct {
	// Name is the name of the source object.
	Name string `json:"name,omitempty"`
	// Namespace is the namespace of the source object.
	Namespace string `json:"namespace,omitempty"`
}

// ResolveReferences returns the objects hw's spec.references point to, keyed by
// reference name. When Namespace is set, only references in that namespace are
// considered. A reference is read only if the allow list matches it or the deny
// list does not; with no deny list configured, an implicit deny-all rule is used
// and can still be overridden by an allow-list match. Denied and unreadable
// references are omitted from the map, reported in the error, and other references are still returned.
func (b *Backend) ResolveReferences(ctx context.Context, hw *tinkerbell.Hardware) (map[string]any, error) {
	logger := logr.FromContextOrDiscard(ctx)
	denylist := b.HardwareReferenceDenyListRules
	if len(denylist) == 0 {
		denylist = []string{denyAllReferences}
	}

	references := make(map[string]any)
	var refErr error
	for refName, rf := range hw.Spec.References {
		var missing []string
		if rf.Name == "" {
			missing = append(missing, "name")
		}
		if rf.Version == "" {
			missing = append(missing, "version")
		}
		if rf.Resource == "" {
			missing = append(missing, "resource")
		}
		if len(missing) > 0 {
			err := fmt.Errorf("reference %q is missing required fields: %s", refName, strings.Join(missing, ", "))
			refErr = errors.Join(refErr, err)
			logger.V(1).Info("incomplete reference", "referenceName", refName, "missingFields", missing)
			continue
		}
		rf.Group = strings.ToLower(rf.Group)
		rf.Resource = strings.ToLower(rf.Resource)
		if b.Namespace != "" && rf.Namespace != b.Namespace {
			err := fmt.Errorf("reference %q namespace %q is outside configured backend namespace %q", refName, rf.Namespace, b.Namespace)
			refErr = errors.Join(refErr, err)
			continue
		}

		ed := evaluationData{
			Source:    source{Name: hw.Name, Namespace: hw.Namespace},
			Reference: rf,
		}
		denied, drules, err := evaluate(ctx, denylist, ed)
		if err != nil {
			refErr = errors.Join(refErr, err)
			logger.V(1).Info("error applying denylist rules", "error", err, "denyRules", denylist)
			continue
		}
		allowed, arules, err := evaluate(ctx, b.HardwareReferenceAllowListRules, ed)
		if err != nil {
			refErr = errors.Join(refErr, err)
			logger.V(1).Info("error applying allowlist rules", "error", err, "allowRules", b.HardwareReferenceAllowListRules)
			continue
		}
		if denied && !allowed {
			refErr = errors.Join(refErr, fmt.Errorf("reference %q denied", refName))
			logger.V(1).Info("reference denied", "referenceName", refName, "denyRules", drules, "allowRules", arules)
			continue
		}
		logger.V(1).Info("reference allowed", "referenceName", refName, "denyRules", drules, "allowRules", arules)
		gvr := schema.GroupVersionResource{Group: rf.Group, Version: rf.Version, Resource: rf.Resource}
		if v, err := b.dynamicRead(ctx, gvr, rf.Name, rf.Namespace); err == nil || v != nil {
			references[refName] = v
		} else {
			refErr = errors.Join(refErr, err)
			logger.V(1).Info("error getting reference", "referenceName", rf.Name, "namespace", rf.Namespace, "gvr", gvr, "error", err, "refNil", v == nil)
		}
	}

	return references, refErr
}

// evaluate checks if the data matches any rules defined.
// It returns a boolean indicating if at least one rule was matched, the rule that matched for the decision, and an error if any occurred.
func evaluate(_ context.Context, rules []string, data evaluationData) (bool, string, error) {
	q, err := quamina.New()
	if err != nil {
		return false, "", fmt.Errorf("error creating rule evaluation engine: %w", err)
	}
	for _, r := range rules {
		if err := q.AddPattern(fmt.Sprintf("pattern-%v", r), r); err != nil {
			return false, "", fmt.Errorf("error adding matching pattern: %v err: %w", r, err)
		}
	}

	jsonEvent, err := json.Marshal(&data)
	if err != nil {
		return false, "", fmt.Errorf("error while marshalling data: %w", err)
	}
	matches, err := q.MatchesForEvent(jsonEvent)
	if err != nil {
		return false, "", fmt.Errorf("error while matching pattern: %w", err)
	}
	if len(matches) == 0 {
		return false, "", nil
	}

	var rs []string
	for idx, match := range matches {
		if m, ok := match.(string); ok {
			rs = append(rs, m)
		} else {
			rs = append(rs, fmt.Sprintf("pattern-%d", idx))
		}
	}

	return true, strings.Join(rs, ";"), nil
}
