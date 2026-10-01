package render

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"text/template"
)

// leaf is a templated string value located in the document.
type leaf struct {
	path string             // dotted/indexed location, e.g. spec.instance.hostname
	segs []string           // unambiguous map keys and slice indexes of path
	src  string             // original template text
	tmpl *template.Template // parsed template
	set  func(any)          // writes the rendered value back into the parent container
	refs []selfRef          // self-reference paths (self key stripped)
	node *graphNode         // dependency graph node
}

// collectLeaves walks the document, appending a leaf for every string value
// that contains the left delimiter and is not skipped. set writes a new value
// into the parent container so the rendered result lands back in the document.
func collectLeaves(node any, path string, segs []string, set func(any), cfg *config, out *[]*leaf) {
	switch n := node.(type) {
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(n)) {
			collectLeaves(n[k], joinKey(path, k), join(segs, k), func(v any) { n[k] = v }, cfg, out)
		}
	case []any:
		for i := range n {
			collectLeaves(n[i], fmt.Sprintf("%s[%d]", path, i), join(segs, strconv.Itoa(i)), func(v any) { n[i] = v }, cfg, out)
		}
	case string:
		if strings.Contains(n, leftDelim) && (cfg.skip == nil || !cfg.skip(path)) {
			*out = append(*out, &leaf{path: path, segs: segs, src: n, set: set})
		}
	}
}

// parse builds the leaf's template and derives its self-references.
func (l *leaf) parse(cfg *config) error {
	t, err := buildTemplate(l.src, cfg)
	if err != nil {
		return &FieldError{Path: l.path, Err: err}
	}
	l.tmpl = t
	l.refs = selfRefs(t, cfg)
	return nil
}

func joinKey(base, key string) string {
	if isPathKey(key) {
		if base == "" {
			return key
		}
		return base + "." + key
	}
	return base + "[" + strconv.Quote(key) + "]"
}

func isPathKey(key string) bool {
	for i, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' ||
			(i > 0 && ((r >= '0' && r <= '9') || r == '-')) {
			continue
		}
		return false
	}
	return key != ""
}
