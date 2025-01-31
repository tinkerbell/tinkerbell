package staticroute

// unorderedSet is a utility data structure that behaves as a traditional unorderedSet. Its elements are unordered.
type unorderedSet map[string]struct{}

func newUnorderedSet() unorderedSet {
	return make(unorderedSet)
}

// Insert adds v to s.
func (s unorderedSet) Insert(v string) {
	s[v] = struct{}{}
}

// Range iterates over the elements in s and calls fn for each element.
func (s unorderedSet) Range(fn func(v string)) {
	for k := range s {
		fn(k)
	}
}
