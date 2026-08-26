package tinkerbell

import "reflect"

// PruneEmpty removes empty subtrees from the collected attributes so a component
// the source never reported serializes as an absent field rather than an empty
// {} object. It is the single normalization step shared by the in-band and
// out-of-band collection paths: each converts its source into an Attributes
// verbatim, then calls this once. The mappers therefore never decide emptiness
// themselves, keeping the "what is empty" rule in one place.
//
// Emptiness is purely structural (the reflect zero value), applied bottom-up:
// nested empties collapse before their parent is evaluated. A non-nil pointer —
// notably ComponentStatus.PostCode set to 0 (a successful POST) — keeps its
// parent non-empty, so a meaningful zero is never mistaken for absent.
func (a *Attributes) PruneEmpty() {
	if a == nil {
		return
	}
	pruneValue(reflect.ValueOf(a).Elem())
}

// pruneValue walks v in place, nil-ing zero-value pointers and dropping
// zero-value struct elements (and empty slices). v must be addressable so those
// fields can be cleared. Unexported fields (e.g. those inside metav1.Time) are
// skipped, which reflect also forbids setting.
func pruneValue(v reflect.Value) {
	switch v.Kind() { //nolint:exhaustive // only the kinds we expect to see in Attributes
	case reflect.Pointer:
		// Only prune pointers to structs. A pointer to a scalar is itself the
		// signal (ComponentStatus.PostCode *int32 pointing at 0 means "POST
		// succeeded", not "absent"), so such pointers are always preserved.
		if v.IsNil() || v.Elem().Kind() != reflect.Struct {
			return
		}
		pruneValue(v.Elem())
		if v.Elem().IsZero() {
			v.SetZero()
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			pruneValue(v.Field(i))
		}
	case reflect.Slice:
		dropZeroElems := isStructLike(v.Type().Elem())
		n := 0
		for i := 0; i < v.Len(); i++ {
			e := v.Index(i)
			pruneValue(e)
			if dropZeroElems && e.IsZero() {
				continue
			}
			if n != i {
				v.Index(n).Set(e)
			}
			n++
		}
		switch {
		case n == 0:
			v.SetZero() // nil the slice so omitempty applies
		case n < v.Len():
			v.Set(v.Slice(0, n))
		}
	}
}

// isStructLike reports whether t is a struct or pointer-to-struct, i.e. an
// element type whose zero value should be dropped from a slice. Scalar elements
// (e.g. capability strings) are kept as reported.
func isStructLike(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct
}
