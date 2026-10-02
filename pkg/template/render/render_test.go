package render_test

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/pkg/template/render"
)

func TestHasTemplates(t *testing.T) {
	for _, test := range []struct {
		name string
		doc  any
		skip func(string) bool
		want bool
	}{
		{name: "nil"},
		{name: "literal", doc: "plain text"},
		{name: "invalid template is detected without parsing", doc: "{{ invalid", want: true},
		{name: "nested", doc: map[string]any{"spec": []any{"literal", "{{ invalid"}}, want: true},
		{name: "skipped", doc: map[string]any{"userData": "{{ ds.meta_data.hostname }}"}, skip: func(path string) bool { return path == "userData" }},
		{name: "array path", doc: []any{"{{ invalid"}, skip: func(path string) bool { return path == "[0]" }},
		{name: "quoted key", doc: map[string]any{"a.b": "{{ invalid"}, skip: func(path string) bool { return path == `["a.b"]` }},
		{name: "mixed", doc: map[string]any{"skip": "{{ invalid", "render": "{{ invalid"}, skip: func(path string) bool { return path == "skip" }, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			orig, err := json.Marshal(test.doc)
			if err != nil {
				t.Fatal(err)
			}
			if got := render.HasTemplates(test.doc, render.WithSkip(test.skip)); got != test.want {
				t.Errorf("HasTemplates = %v, want %v", got, test.want)
			}
			after, err := json.Marshal(test.doc)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(string(orig), string(after)); diff != "" {
				t.Errorf("input was modified (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValue(t *testing.T) {
	tests := map[string]struct {
		doc  map[string]any
		data map[string]any
		opts []render.Option
		want map[string]any
	}{
		"self reference": {
			doc:  map[string]any{"name": "example", "greeting": "hello {{ .self.name }}"},
			want: map[string]any{"name": "example", "greeting": "hello example"},
		},
		"nested and list": {
			doc: map[string]any{"spec": map[string]any{
				"arch":  "x86_64",
				"disks": []any{map[string]any{"name": "{{ .self.spec.arch }}-disk"}},
			}},
			want: map[string]any{"spec": map[string]any{
				"arch":  "x86_64",
				"disks": []any{map[string]any{"name": "x86_64-disk"}},
			}},
		},
		"external data": {
			doc:  map[string]any{"greeting": "hello {{ .env.user }}"},
			data: map[string]any{"env": map[string]any{"user": "tink"}},
			want: map[string]any{"greeting": "hello tink"},
		},
		"chained through external data": {
			doc: map[string]any{
				"arch": "{{ .hw.arch }}",
				"msg":  "arch={{ .self.arch }}",
			},
			data: map[string]any{"hw": map[string]any{"arch": "aarch64"}},
			want: map[string]any{"arch": "aarch64", "msg": "arch=aarch64"},
		},
		"with relative field does not depend on unrelated sibling": {
			doc: map[string]any{
				"a": map[string]any{
					"b": "literal",
					"c": "{{ .self.x }}",
				},
				"x": "{{ with .self.a }}{{ .b }}{{ end }}",
			},
			want: map[string]any{
				"a": map[string]any{
					"b": "literal",
					"c": "literal",
				},
				"x": "literal",
			},
		},
		"if container selector does not depend on descendant values": {
			doc: map[string]any{
				"a": map[string]any{
					"b": "literal",
					"c": "{{ .self.x }}",
				},
				"x": "{{ if .self.a }}yes{{ end }}",
			},
			want: map[string]any{
				"a": map[string]any{
					"b": "literal",
					"c": "yes",
				},
				"x": "yes",
			},
		},
		"range relative field does not depend on unrelated sibling": {
			doc: map[string]any{
				"items": []any{map[string]any{
					"a": "literal",
					"b": "{{ .self.x }}",
				}},
				"x": "{{ range .self.items }}{{ .a }}{{ end }}",
			},
			want: map[string]any{
				"items": []any{map[string]any{
					"a": "literal",
					"b": "literal",
				}},
				"x": "literal",
			},
		},
		"map range relative field does not depend on unrelated sibling": {
			doc: map[string]any{
				"items": map[string]any{"eth0": map[string]any{
					"a": "literal",
					"b": "{{ .self.x }}",
				}},
				"x": "{{ range .self.items }}{{ .a }}{{ end }}",
			},
			want: map[string]any{
				"items": map[string]any{"eth0": map[string]any{
					"a": "literal",
					"b": "literal",
				}},
				"x": "literal",
			},
		},
		"root variable reference inside range tracks dependency": {
			doc: map[string]any{
				"items":   []any{"node1"},
				"prefix":  "{{ .self.cluster }}",
				"cluster": "prod",
				"out":     "{{ range .self.items }}{{ $.self.prefix }}/{{ . }}{{ end }}",
			},
			want: map[string]any{
				"items":   []any{"node1"},
				"prefix":  "prod",
				"cluster": "prod",
				"out":     "prod/node1",
			},
		},
		"static index reads only the selected key": {
			doc: map[string]any{"vars": map[string]any{
				"source": "{{ .self.base }}",
				"target": `{{ index .self.vars "source" }}/x`,
			}, "base": "b"},
			want: map[string]any{"vars": map[string]any{"source": "b", "target": "b/x"}, "base": "b"},
		},
		"dotted key is distinct from nested keys": {
			doc:  map[string]any{"a.b": "{{ .self.a.b }}", "a": map[string]any{"b": "nested"}},
			want: map[string]any{"a.b": "nested", "a": map[string]any{"b": "nested"}},
		},
		"rendered values are always strings": {
			doc:  map[string]any{"n": int64(42), "flag": true, "count": "{{ .self.n }}", "on": "{{ .self.flag }}", "mode": `{{ "0644" }}`},
			want: map[string]any{"n": int64(42), "flag": true, "count": "42", "on": "true", "mode": "0644"},
		},
		"literal delimiter": {
			doc:  map[string]any{"jinja": `{{ "{{" }} ds.meta_data.hostname }}`},
			want: map[string]any{"jinja": "{{ ds.meta_data.hostname }}"},
		},
		"missing key disabled": {
			doc:  map[string]any{"greeting": "hello {{ .self.missing }}"},
			opts: []render.Option{render.WithMissingKeyError(false)},
			want: map[string]any{"greeting": "hello <no value>"},
		},
		"funcs": {
			doc:  map[string]any{"name": "example", "shout": "{{ .self.name | upper }}"},
			opts: []render.Option{render.WithFuncs(template.FuncMap{"upper": strings.ToUpper})},
			want: map[string]any{"name": "example", "shout": "EXAMPLE"},
		},
		"custom self key": {
			doc:  map[string]any{"name": "example", "greeting": "hi {{ .hardware.name }}"},
			opts: []render.Option{render.WithSelfKey("hardware")},
			want: map[string]any{"name": "example", "greeting": "hi example"},
		},
		"skipped values stay unrendered and readable": {
			doc: map[string]any{
				"metadata": map[string]any{"note": "{{ .self.x }}"},
				"copy":     "{{ .self.metadata.note }}",
			},
			opts: []render.Option{render.WithSkip(func(p string) bool { return strings.HasPrefix(p, "metadata.") })},
			want: map[string]any{
				"metadata": map[string]any{"note": "{{ .self.x }}"},
				"copy":     "{{ .self.x }}",
			},
		},
		"skip distinguishes dotted map keys": {
			doc: map[string]any{
				"a.b": "{{ .self.x }}",
				"a":   map[string]any{"b": "{{ .self.x }}"},
				"x":   "rendered",
			},
			opts: []render.Option{render.WithSkip(func(p string) bool { return p == `["a.b"]` })},
			want: map[string]any{
				"a.b": "{{ .self.x }}",
				"a":   map[string]any{"b": "rendered"},
				"x":   "rendered",
			},
		},
		"binary output is preserved": {
			doc:  map[string]any{"der": "{{ raw }}"},
			opts: []render.Option{render.WithFuncs(template.FuncMap{"raw": func() string { return "\x30\x82\x00\xff\xfe" }})},
			want: map[string]any{"der": "\x30\x82\x00\xff\xfe"},
		},
		"no templates": {
			doc:  map[string]any{"a": "b", "n": int64(1)},
			want: map[string]any{"a": "b", "n": int64(1)},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := render.Value(tt.doc, tt.data, tt.opts...)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestCycleErrorOrderIsDeterministic(t *testing.T) {
	doc := map[string]any{"b": "{{ .self.a }}", "a": "{{ .self.b }}"}
	_, err := render.Value(doc, nil)
	var cycle *render.CycleError
	if !errors.As(err, &cycle) {
		t.Fatalf("err = %v, want *CycleError", err)
	}
	if diff := cmp.Diff([]string{"a", "b", "a"}, cycle.Fields); diff != "" {
		t.Errorf("cycle fields (-want +got):\n%s", diff)
	}
}

func TestNotContainerTruthinessDoesNotDependOnChildren(t *testing.T) {
	doc := map[string]any{
		"a": map[string]any{
			"b": "{{ .self.x }}",
		},
		"x": "{{ if not .self.a }}empty{{ else }}full{{ end }}",
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"a": map[string]any{"b": "full"},
		"x": "full",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestNestedLogicalTruthinessDoesNotDependOnChildren(t *testing.T) {
	doc := map[string]any{
		"a": map[string]any{"b": "{{ .self.x }}"},
		"x": "{{ if not (and .self.a) }}empty{{ else }}full{{ end }}",
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"a": map[string]any{"b": "full"},
		"x": "full",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestLogicalPipelineIntoNotDoesNotDependOnChildren(t *testing.T) {
	doc := map[string]any{
		"a": map[string]any{"b": "{{ .self.x }}"},
		"x": "{{ if and .self.a .self.a | not }}empty{{ else }}full{{ end }}",
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["x"] != "full" {
		t.Fatalf("x = %q, want full", got.(map[string]any)["x"])
	}
}

func TestFieldOfLogicalResultWaitsForSelectedValue(t *testing.T) {
	doc := map[string]any{
		"out": `{{ not (or .self.p).x }}`,
		"p":   map[string]any{"x": `{{ "" }}`},
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "true" {
		t.Fatalf("out = %q, want true", got.(map[string]any)["out"])
	}
}

func TestIfAssignmentWaitsForReturnedAndOrValue(t *testing.T) {
	doc := map[string]any{
		"base": "rendered",
		"out":  `{{ if $x := or .self.z .self.base }}{{ $x.name }}{{ end }}`,
		"z":    map[string]any{"name": "{{ .self.base }}"},
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "rendered" {
		t.Fatalf("out = %q, want rendered", got.(map[string]any)["out"])
	}
}

func TestShortCircuitAndOrDoesNotCommitSkippedArgumentAssignments(t *testing.T) {
	doc := map[string]any{
		"a":   map[string]any{"c": "literal"},
		"z":   map[string]any{"c": `{{ "rendered" }}`},
		"out": `{{ $x := .self.z }}{{ or 1 ($x := .self.a) }}{{ $x.c }}`,
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "1rendered" {
		t.Fatalf("out = %q, want 1rendered", got.(map[string]any)["out"])
	}
}

func TestShadowedAssignmentDoesNotMakeOuterIndexDynamic(t *testing.T) {
	doc := map[string]any{
		"vars": map[string]any{
			"a": "OK",
			"b": "{{ .self.out }}",
		},
		"out": `{{ $k := "a" }}{{ with .env }}{{ $k := "b" }}{{ $k = "b" }}{{ end }}{{ index .self.vars $k }}`,
	}
	data := map[string]any{"env": map[string]any{"present": true}}

	got, err := render.Value(doc, data)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "OK" {
		t.Fatalf("out = %q, want OK", got.(map[string]any)["out"])
	}
}

func TestConditionalDeclarationUpdatesIndexBinding(t *testing.T) {
	doc := map[string]any{
		"a": `{{ $k := "x" }}{{ if and .self.c ($k := "z") }}{{ index .self $k }}{{ end }}`,
		"c": true,
		"x": "X",
		"z": "{{ .self.x }}!",
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["a"] != "X!" {
		t.Fatalf("a = %q, want X!", got.(map[string]any)["a"])
	}
}

func TestMergedIndexChoicesAreTrackedByTruthinessChecks(t *testing.T) {
	doc := map[string]any{
		"c":   "yes",
		"out": `{{ $k := "a" }}{{ or .self.c ($k := "b") }}{{ if index .self.z $k }}T{{ else }}F{{ end }}`,
		"z":   map[string]any{"a": `{{ "" }}`, "b": "B"},
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "yesF" {
		t.Fatalf("out = %q, want yesF", got.(map[string]any)["out"])
	}
}

func TestAssignmentToMergedIndexBindingTracksNewKey(t *testing.T) {
	doc := map[string]any{
		"c":   "yes",
		"out": `{{ $k := "x" }}{{ $_ := and .self.c ($k := "z") }}{{ if .self.c }}{{ $k = "q" }}{{ end }}{{ index .self.m $k }}`,
		"m": map[string]any{
			"q": "{{ .self.m.x }}!",
			"x": "X",
			"z": "Z",
		},
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "X!" {
		t.Fatalf("out = %q, want X!", got.(map[string]any)["out"])
	}
}

func TestRangeAssignmentAfterMergedKeyTracksLaterIteration(t *testing.T) {
	doc := map[string]any{
		"a":     `{{ $k := "a" }}{{ range .self.items }}{{ index $.self.m $k }}{{ $_ := and $.self.c ($k := "x") }}{{ $k = "b" }}{{ end }}`,
		"c":     "",
		"items": []any{1, 2},
		"m": map[string]any{
			"a": "A",
			"b": "{{ .self.m.a }}!",
		},
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["a"] != "AA!" {
		t.Fatalf("a = %q, want AA!", got.(map[string]any)["a"])
	}
}

func TestRangeValueVariableTracksElementFields(t *testing.T) {
	doc := map[string]any{
		"a":     `{{ range $item := .self.items }}{{ $item.name }}{{ end }}`,
		"base":  "OK",
		"items": []any{map[string]any{"name": "{{ .self.base }}"}},
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["a"] != "OK" {
		t.Fatalf("a = %q, want OK", got.(map[string]any)["a"])
	}
}

func TestRangeVariableDoesNotLeakAfterBlock(t *testing.T) {
	doc := map[string]any{
		"items": []any{1},
		"out":   `{{ $item := .self.other }}{{ range $item := .self.items }}{{ end }}{{ $item.name }}`,
		"other": map[string]any{"name": "OK"},
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "OK" {
		t.Fatalf("out = %q, want OK", got.(map[string]any)["out"])
	}
}

func TestRangeCarriedMergedBindingTracksReassignment(t *testing.T) {
	doc := map[string]any{
		"c":     "",
		"items": []any{1, 2},
		"out":   `{{ $k := "a" }}{{ range $.self.items }}{{ index $.self.z $k }}{{ $_ := and $.self.c ($k := "a") }}{{ $k = "b" }}{{ end }}`,
		"z":     map[string]any{"a": "A", "b": "{{ .self.z.a }}!"},
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "AA!" {
		t.Fatalf("out = %q, want AA!", got.(map[string]any)["out"])
	}
}

func TestChoiceCapKeepsDependenciesOfSourceValues(t *testing.T) {
	doc := map[string]any{"src": `{{ "a" }}`, "z": map[string]any{"a": `{{ "A" }}`}}
	var tmpl strings.Builder
	tmpl.WriteString(`{{ $k := .self.src }}`)
	for i := range 17 {
		name := "p" + strconv.Itoa(i)
		doc[name] = false
		tmpl.WriteString(`{{ if .self.` + name + ` }}{{ $k = "unused" }}{{ end }}`)
	}
	tmpl.WriteString(`{{ index .self.z $k }}`)
	doc["out"] = tmpl.String()

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "A" {
		t.Fatalf("out = %q, want A", got.(map[string]any)["out"])
	}
}

func TestIndexChoiceCapKeepsCandidateDependencies(t *testing.T) {
	doc := map[string]any{
		"c":  "yes",
		"x1": map[string]any{"k0": `{{ "key" }}`},
		"x2": map[string]any{"k0": "key"},
		"z":  map[string]any{"key": "OK"},
	}
	var tmpl strings.Builder
	tmpl.WriteString(`{{ $m := .self.x1 }}{{ $_ := or .self.c ($m := .self.x2) }}{{ $k := "k0" }}`)
	for i := 1; i <= 8; i++ {
		tmpl.WriteString(`{{ $_ := or .self.c ($k := "k` + strconv.Itoa(i) + `") }}`)
	}
	tmpl.WriteString(`{{ index .self.z (index $m $k) }}`)
	doc["out"] = tmpl.String()

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "OK" {
		t.Fatalf("out = %q, want OK", got.(map[string]any)["out"])
	}
}

func TestLiteralShortCircuitSkipsAssignmentToIndexBinding(t *testing.T) {
	doc := map[string]any{
		"vars": map[string]any{"a": "OK", "b": "{{ .self.out }}"},
		"out":  `{{ $k := "a" }}{{ or true ($k = "b") }}{{ index .self.vars $k }}`,
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "trueOK" {
		t.Fatalf("out = %q, want trueOK", got.(map[string]any)["out"])
	}
}

func TestLiteralIfSkipsAssignmentToIndexBinding(t *testing.T) {
	doc := map[string]any{
		"vars": map[string]any{"a": "OK", "b": "{{ .self.out }}"},
		"out":  `{{ $k := "a" }}{{ if false }}{{ $k = .self.vars.b }}{{ end }}{{ index .self.vars $k }}`,
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "OK" {
		t.Fatalf("out = %q, want OK", got.(map[string]any)["out"])
	}
}

func TestOverriddenOrEvaluatesAndTracksEveryArgument(t *testing.T) {
	doc := map[string]any{
		"c":   true,
		"m":   map[string]any{"a": "A", "b": "{{ .self.m.a }}!"},
		"out": `{{ $k := "a" }}{{ if .self.c }}{{ $_ := or true ($k = "b") }}{{ end }}{{ index .self.m $k }}`,
	}
	or := func(args ...any) any { return args[len(args)-1] }

	got, err := render.Value(doc, nil, render.WithFuncs(template.FuncMap{"or": or}))
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "A!" {
		t.Fatalf("out = %q, want A!", got.(map[string]any)["out"])
	}
}

func TestShortCircuitOrSkipsLiteralUnreachableReference(t *testing.T) {
	doc := map[string]any{
		"a":   "{{ .self.out }}",
		"out": "{{ if or true .self.a }}yes{{ end }}",
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "yes" || got.(map[string]any)["a"] != "yes" {
		t.Fatalf("rendered document = %#v, want both fields to be yes", got)
	}
}

func TestUnusedIfLogicalDeclarationReadsOnlyTruthiness(t *testing.T) {
	doc := map[string]any{
		"a":   map[string]any{"b": "{{ .self.out }}"},
		"out": "{{ if $x := or .self.a true }}yes{{ end }}",
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "yes" || got.(map[string]any)["a"].(map[string]any)["b"] != "yes" {
		t.Fatalf("rendered document = %#v, want out and a.b to be yes", got)
	}
}

func TestLogicalArgumentDeclarationIsKnownAfterExecution(t *testing.T) {
	got, err := render.Value(map[string]any{
		"out": `{{ and 1 ($x := "k") }}{{ $x }}`,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "kk" {
		t.Fatalf("out = %q, want kk", got.(map[string]any)["out"])
	}
}

func TestIfLogicalDeclarationUsedInNestedConditionKeepsDependencies(t *testing.T) {
	doc := map[string]any{
		"a":   map[string]any{"c": "x"},
		"out": `{{ if $x := and .self.a .self.z }}{{ if $x.c }}yes{{ else }}no{{ end }}{{ end }}`,
		"z":   map[string]any{"c": `{{ "" }}`},
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "no" {
		t.Fatalf("out = %q, want no", got.(map[string]any)["out"])
	}
}

func TestShadowedIfDeclarationDoesNotCreateCycle(t *testing.T) {
	doc := map[string]any{
		"a":   map[string]any{"b": "{{ .self.out }}"},
		"out": `{{ if $x := .self.a }}{{ with $x := .env }}{{ $x.v }}{{ end }}{{ end }}`,
	}
	data := map[string]any{"env": map[string]any{"v": "ok"}}

	got, err := render.Value(doc, data)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "ok" || got.(map[string]any)["a"].(map[string]any)["b"] != "ok" {
		t.Fatalf("rendered document = %#v, want out and a.b to be ok", got)
	}
}

func TestPipedNotAfterLogicalValueUsesOnlyTruthiness(t *testing.T) {
	doc := map[string]any{
		"a":   map[string]any{"b": "{{ .self.out }}"},
		"out": "{{ and .self.a .self.a | not }}",
	}

	got, err := render.Value(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["out"] != "false" || got.(map[string]any)["a"].(map[string]any)["b"] != "false" {
		t.Fatalf("rendered document = %#v, want both fields false", got)
	}
}

func TestCycleErrorClosesWhenGroupNodeClosesCycle(t *testing.T) {
	doc := map[string]any{
		"_c": "{{ .self.a }}",
		"a":  map[string]any{"x": "{{ .self.b }}"},
		"b":  map[string]any{"y": "{{ .self.a }}"},
	}
	_, err := render.Value(doc, nil)
	var cycle *render.CycleError
	if !errors.As(err, &cycle) {
		t.Fatalf("err = %v, want *CycleError", err)
	}
	if diff := cmp.Diff([]string{"a.x", "b.y", "a.x"}, cycle.Fields); diff != "" {
		t.Errorf("cycle fields (-want +got):\n%s", diff)
	}
}

func TestValueDataIsNotTemplated(t *testing.T) {
	data := map[string]any{"ref": map[string]any{"v": "{{ .self.secret }}"}}
	doc := map[string]any{"secret": "s", "out": "{{ .ref.v }}"}

	got, err := render.Value(doc, data)
	if err != nil {
		t.Fatal(err)
	}
	if out := got.(map[string]any)["out"]; out != "{{ .self.secret }}" {
		t.Fatalf("out = %q, want the data value verbatim", out)
	}
}

func TestValueErrors(t *testing.T) {
	balloon := template.FuncMap{"balloon": func() string { return strings.Repeat("x", 1<<20) }}

	tests := map[string]struct {
		doc      map[string]any
		data     map[string]any
		opts     []render.Option
		wantIs   error
		wantPath string
	}{
		"cycle": {
			doc:    map[string]any{"a": "{{ .self.b }}", "b": "{{ .self.a }}"},
			wantIs: render.ErrReferenceCycle,
		},
		"direct self cycle": {
			doc:    map[string]any{"a": "{{ .self.a }}"},
			wantIs: render.ErrReferenceCycle,
		},
		"missing key": {
			doc:      map[string]any{"greeting": "hello {{ .self.missing }}"},
			wantPath: "greeting",
		},
		"parse error": {
			doc:      map[string]any{"list": []any{"{{ .self.x "}},
			wantPath: "list[0]",
		},
		"output too large": {
			doc:      map[string]any{"big": "{{ balloon }}"},
			opts:     []render.Option{render.WithFuncs(balloon), render.WithMaxOutputBytes(1024)},
			wantIs:   render.ErrOutputTooLarge,
			wantPath: "big",
		},
		"total output too large": {
			doc: map[string]any{"a": "{{ balloon }}", "b": "{{ balloon }}", "c": "{{ balloon }}"},
			opts: []render.Option{
				render.WithFuncs(balloon),
				render.WithMaxOutputBytes(0),
				render.WithMaxTotalBytes(2 << 20),
			},
			wantIs: render.ErrTotalOutputTooLarge,
		},
		"output deadline": {
			doc:      map[string]any{"spin": "{{ range .items }}{{ . }}{{ end }}"},
			data:     map[string]any{"items": make([]int, 5_000_000)},
			opts:     []render.Option{render.WithOutputDeadline(time.Millisecond)},
			wantIs:   render.ErrOutputDeadline,
			wantPath: "spin",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := render.Value(tt.doc, tt.data, tt.opts...)
			if err == nil {
				t.Fatal("expected an error")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("err = %v, want %v", err, tt.wantIs)
			}
			if tt.wantPath != "" {
				var fe *render.FieldError
				if !errors.As(err, &fe) || fe.Path != tt.wantPath {
					t.Errorf("err = %v, want *FieldError at %q", err, tt.wantPath)
				}
			}
		})
	}
}

func TestValueOutputCapDisabled(t *testing.T) {
	funcs := template.FuncMap{"balloon": func() string { return strings.Repeat("x", 4096) }}
	got, err := render.Value(map[string]any{"big": "{{ balloon }}"}, nil,
		render.WithFuncs(funcs), render.WithMaxOutputBytes(0), render.WithMaxTotalBytes(0))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.(map[string]any)["big"].(string)); n != 4096 {
		t.Fatalf("len = %d, want 4096", n)
	}
}
