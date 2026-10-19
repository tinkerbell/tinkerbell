package delimitedlist

import (
	"slices"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSpaceList(t *testing.T) {
	tests := map[string]struct {
		input    string
		want     []string
		source   string // "Set", "FromEnv", or "FromFile"
		wantStr  string
		initWith []string // initial values if any
	}{
		"single value": {
			input:   "value1",
			want:    []string{"value1"},
			source:  "Set",
			wantStr: "value1",
		},
		"multiple values": {
			input:   "value1 value2 value3",
			want:    []string{"value1", "value2", "value3"},
			source:  "Set",
			wantStr: "value1 value2 value3",
		},
		"multiple values with extra spaces": {
			input:   "  value1   value2    value3  ",
			want:    []string{"value1", "value2", "value3"},
			source:  "Set",
			wantStr: "value1 value2 value3",
		},
		"from environment": {
			input:   "env1 env2",
			want:    []string{"env1", "env2"},
			source:  "FromEnv",
			wantStr: "env1 env2",
		},
		"from file": {
			input:   "file1 file2",
			want:    []string{"file1", "file2"},
			source:  "FromFile",
			wantStr: "file1 file2",
		},
		"append to existing values": {
			input:    "value3 value4",
			initWith: []string{"value1", "value2"},
			want:     []string{"value1", "value2", "value3", "value4"},
			source:   "Set",
			wantStr:  "value1 value2 value3 value4",
		},
		"empty input": {
			input:   "",
			want:    nil,
			source:  "Set",
			wantStr: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var target []string
			if tt.initWith != nil {
				target = append(target, tt.initWith...)
			}

			v := New(&target, ' ')

			var err error
			switch tt.source {
			case "FromEnv":
				err = v.FromEnv(tt.input)
			case "FromFile":
				err = v.FromFile(tt.input)
			default:
				err = v.Set(tt.input)
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(target, tt.want); diff != "" {
				t.Errorf("values mismatch (-got +want):\n%s", diff)
			}

			if diff := cmp.Diff(v.String(), tt.wantStr); diff != "" {
				t.Errorf("String() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestParsedValue(t *testing.T) {
	target := []int{9}
	v := NewParsed(&target, ',', strconv.Atoi)

	if err := v.Set(" 1, , 2 "); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]int{1, 2}, target); diff != "" {
		t.Fatalf("values mismatch (-want +got):\n%s", diff)
	}
	if got, want := v.String(), "1,2"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	if err := v.Set("3,invalid"); err == nil {
		t.Fatal("expected parse error")
	}
	if diff := cmp.Diff([]int{1, 2}, target); diff != "" {
		t.Fatalf("parse failure mutated values (-want +got):\n%s", diff)
	}

	if err := v.Set("4"); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]int{4}, target); diff != "" {
		t.Fatalf("replacement mismatch (-want +got):\n%s", diff)
	}

	if err := v.Reset(); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]int{9}, target); diff != "" {
		t.Fatalf("Reset() did not restore initial values (-want +got):\n%s", diff)
	}

	target[0] = 10
	if err := v.Reset(); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]int{9}, target); diff != "" {
		t.Fatalf("Reset() reused a mutated slice (-want +got):\n%s", diff)
	}
}

func TestParsedValueOptions(t *testing.T) {
	tests := map[string]struct {
		initial []int
		opts    []ParsedOption[int]
		sets    []string
		want    []int
		wantErr bool
	}{
		"default replaces": {
			sets: []string{"1,2", "3"},
			want: []int{3},
		},
		"accumulate appends": {
			opts: []ParsedOption[int]{Accumulate[int]()},
			sets: []string{"1,2", "3"},
			want: []int{1, 2, 3},
		},
		"accumulate keeps initial": {
			initial: []int{9},
			opts:    []ParsedOption[int]{Accumulate[int]()},
			sets:    []string{"1"},
			want:    []int{9, 1},
		},
		"dedupe within one set": {
			opts: []ParsedOption[int]{Dedupe[int]()},
			sets: []string{"1,2,1"},
			want: []int{1, 2},
		},
		"accumulate and dedupe across sets": {
			opts: []ParsedOption[int]{Accumulate[int](), Dedupe[int]()},
			sets: []string{"1,2", "2,3", "1"},
			want: []int{1, 2, 3},
		},
		"accumulate failure is atomic": {
			opts:    []ParsedOption[int]{Accumulate[int]()},
			sets:    []string{"1", "2,invalid"},
			want:    []int{1},
			wantErr: true,
		},
		"nil option ignored": {
			opts: []ParsedOption[int]{nil, Accumulate[int]()},
			sets: []string{"1", "2"},
			want: []int{1, 2},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			target := slices.Clone(tt.initial)
			v := NewParsed(&target, ',', strconv.Atoi, tt.opts...)
			var err error
			for _, s := range tt.sets {
				if err = v.Set(s); err != nil {
					break
				}
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("Set() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, target); diff != "" {
				t.Fatalf("values mismatch (-want +got):\n%s", diff)
			}
			if err := v.Reset(); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.initial, target, cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("Reset() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParsedValuePlaceholder(t *testing.T) {
	var target []string
	parse := func(s string) (string, error) { return s, nil }
	if got := NewParsed(&target, ',', parse).GetPlaceholder(); got != "" {
		t.Errorf("default GetPlaceholder() = %q, want empty", got)
	}
	if got := NewParsed(&target, ',', parse, Placeholder[string]("ITEMS")).GetPlaceholder(); got != "ITEMS" {
		t.Errorf("GetPlaceholder() = %q, want %q", got, "ITEMS")
	}
}

func TestString_NilTarget(t *testing.T) {
	tests := map[string]struct {
		target   *[]string
		expected string
	}{
		"nil target": {
			target:   nil,
			expected: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			v := &Value{target: tt.target}
			if diff := cmp.Diff(v.String(), tt.expected); diff != "" {
				t.Errorf("String() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}
