package render

import (
	"errors"
	"strconv"
	"testing"
	"text/template"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSelfRefs(t *testing.T) {
	tests := map[string]struct {
		template string
		funcs    template.FuncMap
		want     []selfRef
	}{
		"with relative field": {
			template: `{{ with .self.a }}{{ .b }}{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}, {path: []string{"a", "b"}}},
		},
		"if selector is exact": {
			template: `{{ if .self.a }}yes{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}},
		},
		"range relative field": {
			template: `{{ range .self.items }}{{ .a }}{{ end }}`,
			want:     []selfRef{{path: []string{"items"}, exact: true}, {path: []string{"items", wildcard, "a"}}},
		},
		"root variable inside range": {
			template: `{{ range .self.items }}{{ $.self.prefix }}{{ end }}`,
			want:     []selfRef{{path: []string{"items"}, exact: true}, {path: []string{"prefix"}}},
		},
		"root alias tracks selected field": {
			template: `{{ $root := . }}{{ $root.self.prefix }}`,
			want:     []selfRef{{path: []string{"prefix"}}},
		},
		"root alias tracks only the selected field": {
			template: `{{ $root := . }}{{ with $root.self.a }}{{ $root.self.x }}{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}, {path: []string{"x"}}},
		},
		"static index selects the key": {
			template: `{{ index .self.vars "source" 0 }}`,
			want:     []selfRef{{path: []string{"vars", "source", "0"}}},
		},
		"index with a dynamic key reads any element": {
			template: `{{ index .self.vars .key }}`,
			want:     []selfRef{{path: []string{"vars", wildcard}}},
		},
		"index through the root": {
			template: `{{ index . "self" "a" }}`,
			want:     []selfRef{{path: []string{"a"}}},
		},
		"parenthesized chain": {
			template: `{{ (.self.spec).instance.hostname }}`,
			want:     []selfRef{{path: []string{"spec", "instance", "hostname"}}},
		},
		"len reads only the container": {
			template: `{{ .self.items | len }}`,
			want:     []selfRef{{path: []string{"items"}, exact: true}},
		},
		"function arguments are read in full": {
			template: `{{ printf "%v" .self.a }}`,
			want:     []selfRef{{path: []string{"a"}}},
		},
		"if over the root reads nothing": {
			template: `{{ if $ }}yes{{ end }}`,
		},
		"named template reads its whole argument": {
			template: `{{ define "h" }}{{ .x }}{{ end }}{{ template "h" .self.spec }}`,
			want:     []selfRef{{path: []string{"spec"}}},
		},
		"selector assignment reads its value": {
			template: `{{ $x := .env }}{{ if $x = index .self 1 }}{{ end }}{{ $x.v }}`,
			want:     []selfRef{{path: []string{"1"}}, {path: []string{"1"}, exact: true}, {path: []string{"1", "v"}}},
		},
		"body declaration does not leak into else": {
			template: `{{ $x := .self.a }}{{ if .flag }}{{ $x := .env }}{{ else }}{{ $x.v }}{{ end }}`,
			want:     []selfRef{{path: []string{"a", "v"}}},
		},
		"overridden index reads the container": {
			template: `{{ index .self.vars "source" }}`,
			funcs:    template.FuncMap{"index": func(any, string) string { return "" }},
			want:     []selfRef{{path: []string{"vars"}}},
		},
		"with over the root keeps the root as dot": {
			template: `{{ with $ }}{{ .self.a }}{{ end }}`,
			want:     []selfRef{{path: []string{"a"}}},
		},
		"assignment reads its value": {
			template: `{{ $x := .self.a }}{{ with .self.z }}{{ $x = $.self.b }}{{ end }}{{ $x.c }}`,
			want:     []selfRef{{path: []string{"z"}, exact: true}, {path: []string{"b"}}, {path: []string{"b", "c"}}, {path: []string{"a", "c"}}},
		},
		"reassigned index variable is dynamic": {
			template: `{{ $k := "a" }}{{ if true }}{{ $k = "b" }}{{ end }}{{ index .self.vars $k }}`,
			want:     []selfRef{{path: []string{"vars", wildcard}}},
		},
		"not reads only truthiness": {
			template: `{{ if not .self.a }}empty{{ else }}full{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}},
		},
		"and condition reads only truthiness": {
			template: `{{ if and .self.a .self.b }}yes{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}, {path: []string{"b"}, exact: true}},
		},
		"parenthesized and condition reads only truthiness": {
			template: `{{ if (and .self.a) }}yes{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}},
		},
		"or condition reads only truthiness": {
			template: `{{ if .self.a | or .self.b }}yes{{ end }}`,
			want:     []selfRef{{path: []string{"b"}, exact: true}, {path: []string{"a"}, exact: true}},
		},
		"logical pipeline into not reads only truthiness": {
			template: `{{ if and .self.a .self.a | not }}yes{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}, {path: []string{"a"}, exact: true}},
		},
		"literal or skips later dependency": {
			template: `{{ if or true .self.a }}yes{{ end }}`,
		},
		"if declaration uses logical result only for truthiness": {
			template: `{{ if $x := or .self.a true }}yes{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}},
		},
		"nested branch condition uses if declaration": {
			template: `{{ if $x := and .self.a .self.z }}{{ if $x.c }}yes{{ end }}{{ end }}`,
			want:     []selfRef{{path: []string{"a"}}, {path: []string{"z"}}},
		},
		"shadowed variable does not count as outer use": {
			template: `{{ if $x := .self.a }}{{ with $x := .env }}{{ $x.v }}{{ end }}{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}},
		},
		"and-or result is read when assigned": {
			template: `{{ if $x := or .self.a .self.b }}{{ $x.name }}{{ end }}`,
			want:     []selfRef{{path: []string{"a"}}, {path: []string{"b"}}},
		},
		"and-or intermediate result is read": {
			template: `{{ if or .self.a "" | printf "%v" }}yes{{ end }}`,
			want:     []selfRef{{path: []string{"a"}}},
		},
		"nested and under not reads only truthiness": {
			template: `{{ if not (and .self.a) }}empty{{ else }}full{{ end }}`,
			want:     []selfRef{{path: []string{"a"}, exact: true}},
		},
		"field of logical result reads selected value": {
			template: `{{ not (or .self.p).x }}`,
			want:     []selfRef{{path: []string{"p"}}},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := &config{funcs: tt.funcs, missingKeyErr: true, selfKey: "self"}
			tmpl, err := buildTemplate(tt.template, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, selfRefs(tmpl, cfg), cmp.AllowUnexported(selfRef{}), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildDepsSharesSubtreeDependencies(t *testing.T) {
	readerA := &leaf{path: "out.a", segs: []string{"out", "a"}, refs: []selfRef{{path: []string{"items", wildcard}}}}
	readerB := &leaf{path: "out.b", segs: []string{"out", "b"}, refs: []selfRef{{path: []string{"items", wildcard}}}}
	itemA := &leaf{path: "items.a", segs: []string{"items", "a"}}
	itemB := &leaf{path: "items.b", segs: []string{"items", "b"}}
	buildDeps([]*leaf{readerA, readerB, itemA, itemB})

	if len(readerA.node.deps) != 1 || len(readerB.node.deps) != 1 {
		t.Fatalf("reader dependencies = %d and %d, want one shared subtree each", len(readerA.node.deps), len(readerB.node.deps))
	}
	if readerA.node.deps[0] != readerB.node.deps[0] {
		t.Fatal("readers do not share a subtree dependency node")
	}
	if got := len(readerA.node.deps[0].deps); got != 2 {
		t.Fatalf("subtree dependencies = %d, want two child groups", got)
	}
}

func TestMergeValuesCapsAlternativeCount(t *testing.T) {
	var merged value
	hasMerged := false
	for i := range maxValueChoices + 1 {
		merged = mergeValues(merged, value{key: strconv.Itoa(i), literal: true}, hasMerged)
		hasMerged = true
	}
	if !merged.unknown {
		t.Fatalf("merged value = %#v, want conservative unknown", merged)
	}
}

func TestPathTreeMatch(t *testing.T) {
	leaves := map[string]*leaf{
		"items[0].a":   {segs: []string{"items", "0", "a"}},
		"items.eth0.a": {segs: []string{"items", "eth0", "a"}},
		"other.eth0.a": {segs: []string{"other", "eth0", "a"}},
		`"a.b"`:        {segs: []string{"a.b"}},
		"a":            {segs: []string{"a"}},
	}
	tree := &pathTree{}
	for _, l := range leaves {
		l.node = &graphNode{leaf: l}
		tree.insert(l)
	}
	tree.buildGroup()
	tests := map[string]struct {
		path  []string
		exact bool
		want  []string
	}{
		"wildcard matches slices and maps": {path: []string{"items", wildcard, "a"}, want: []string{"items[0].a", "items.eth0.a"}},
		"dotted key is one segment":        {path: []string{"a.b"}, want: []string{`"a.b"`}},
		"ancestor leaf is read into":       {path: []string{"a", "b"}, want: []string{"a"}},
		"exact excludes descendants":       {path: []string{"items"}, exact: true},
		"descendants":                      {path: []string{"other"}, want: []string{"other.eth0.a"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got []string
			var visit func(*graphNode)
			visit = func(node *graphNode) {
				if node.leaf != nil {
					for name, want := range leaves {
						if node.leaf == want {
							got = append(got, name)
						}
					}
				}
				for _, dep := range node.deps {
					visit(dep)
				}
			}
			tree.match(tt.path, tt.exact, visit)
			if diff := cmp.Diff(tt.want, got, cmpopts.SortSlices(func(a, b string) bool { return a < b }), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestDirectSelfReferenceIsCycle(t *testing.T) {
	cfg := &config{missingKeyErr: true, selfKey: "self"}
	tmpl, err := buildTemplate(`{{ .self.a }}`, cfg)
	if err != nil {
		t.Fatal(err)
	}
	field := &leaf{path: "a", segs: []string{"a"}, refs: selfRefs(tmpl, cfg)}
	buildDeps([]*leaf{field})
	if _, err := topoSort([]*leaf{field}); !errors.Is(err, ErrReferenceCycle) {
		t.Fatalf("topoSort err = %v, want reference cycle", err)
	}
}
