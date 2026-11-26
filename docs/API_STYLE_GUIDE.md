# Kubernetes API Style Guide

These rules apply only to the Kubernetes API types in the `api` module, the Go types that define our CRDs. They follow from how the Kubernetes API server serializes, defaults, and merges objects. They aren't general Go guidance and don't apply to other Go code.

They build on the Kubernetes API conventions [1], which apply wherever this guide is silent. The rules here record only where we made a choice the conventions leave open, where we deviate from them, and conventions that are easy to miss.

1. Field names are camelCase, in Go and in JSON; never snake_case
   - Initialisms keep their Go spelling in JSON: `lookupID`, `isoURL`. This deviates from the conventions, which lowercase them (`lookupId`)
   - Don't abbreviate: `serialNumber`, not `serialNum`
   - Durations are integers with the unit in the name: `leaseTimeSeconds`
2. Integers are `int32` or `int64`; never `int` or unsigned types
3. Every field is marked `+optional` or `+required`. Optional structs are pointers
4. Every list has a `+listType`. Status conditions are `[]metav1.Condition` with `+listType=map` and `+listMapKey=type`
5. A map of subobjects is fine when its key is natural and unique, such as a MAC address. This deviates from the conventions' preference for lists, and lets templates index entries by key: `{{ index .hardware.spec.networkInterfaces "52:54:00:41:05:c6" }}`
6. Fields that point to another object (*Ref fields and the entries in the Hardware References map, for example) have no namespace field; the object resolves in the namespace of the object that declares it.
   - Otherwise anyone who can create the referring object could have a controller read objects in namespaces they can't access
   - References to arbitrary kinds pin `version` instead of resolving the cluster's preferred version, because templates depend on the referent's schema. A removed version then fails loudly instead of rendering wrong values
7. Think twice about `bool` fields. If a third option might ever be needed, a string enum is easier to extend
8. Name a `bool` field so that `false` is the default and the safe choice
   - Opt-in features end in `Enabled`: `enrollmentEnabled`
   - Opt-out features end in `Disabled`: `workflowsDisabled`, or just `disabled` when the parent struct names the subject, as in `netboot.disabled`
   - Use the past participle as a suffix, not an imperative verb as a prefix: `etagMatchDisabled`, not `disableEtagMatch`
   - Code that doesn't handle an unset field then still gets the default behavior instead of the opposite
9. Optional `bool` fields are `*bool` with `omitempty`
   - Unset is equivalent to `false` unless the field's doc comment says otherwise
   - Don't add `+kubebuilder:default=false`. The API server would replace unset with `false`, losing the difference between "not specified" and "explicitly false"
   - Keeping that difference lets server-side apply own an explicit `false`, and lets a later API version give unset its own meaning, such as inheriting a global setting

---

[1]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
