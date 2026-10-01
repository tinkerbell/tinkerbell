package render

import "maps"

// Value renders, in place, the templated string values of doc, a
// document decoded into map[string]any, []any and scalars, and returns it.
//
// doc is exposed to its own templates under the self key and data is merged
// into the same root. It returns an error that is ErrReferenceCycle if fields
// reference each other in a cycle, and a *FieldError if a specific field fails
// to parse or execute. On error, doc may be partially rendered.
func Value(doc any, data map[string]any, opts ...Option) (any, error) {
	cfg := defaults()
	for _, o := range opts {
		o(&cfg)
	}

	var leaves []*leaf
	collectLeaves(doc, "", nil, func(v any) { doc = v }, &cfg, &leaves)
	if len(leaves) == 0 {
		return doc, nil
	}

	for _, l := range leaves {
		if err := l.parse(&cfg); err != nil {
			return nil, err
		}
	}
	buildDeps(leaves)
	order, err := topoSort(leaves)
	if err != nil {
		return nil, err
	}

	root := make(map[string]any, len(data)+1)
	maps.Copy(root, data)
	root[cfg.selfKey] = doc

	var total *int
	if cfg.maxTotalBytes > 0 {
		total = &cfg.maxTotalBytes
	}
	for _, l := range order {
		s, err := execTemplate(l.tmpl, root, total, &cfg)
		if err != nil {
			return nil, &FieldError{Path: l.path, Err: err}
		}
		l.set(s)
	}

	return doc, nil
}
