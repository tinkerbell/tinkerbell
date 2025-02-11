package staticroute_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/ec2/internal/staticroute"
)

func TestBuilder(t *testing.T) {
	cases := []struct {
		Name      string
		Endpoints []string
		Routes    []staticroute.Route
	}{
		{
			Name:      "NoEndpoints",
			Endpoints: []string{},
			Routes:    nil,
		},
		{
			Name:      "MissingLeadingSlash",
			Endpoints: []string{"foo/bar"},
			Routes: []staticroute.Route{
				{
					Endpoint: "",
					Children: []string{"foo/"},
				},
				{
					Endpoint: "/foo",
					Children: []string{"bar"},
				},
			},
		},
		{
			Name:      "SingleEndpoint",
			Endpoints: []string{"/foo/bar"},
			Routes: []staticroute.Route{
				{
					Endpoint: "",
					Children: []string{"foo/"},
				},
				{
					Endpoint: "/foo",
					Children: []string{"bar"},
				},
			},
		},
		{
			Name:      "NestedEndpoints",
			Endpoints: []string{"/foo/bar", "/foo/bar/baz"},
			Routes: []staticroute.Route{
				{
					Endpoint: "",
					Children: []string{"foo/"},
				},
				{
					Endpoint: "/foo",
					Children: []string{"bar/"},
				},
				{
					Endpoint: "/foo/bar",
					Children: []string{"baz"},
				},
			},
		},

		{
			Name:      "DeepNestedEndpoints",
			Endpoints: []string{"/foo/bar/baz/qux"},
			Routes: []staticroute.Route{
				{
					Endpoint: "",
					Children: []string{"foo/"},
				},
				{
					Endpoint: "/foo",
					Children: []string{"bar/"},
				},
				{
					Endpoint: "/foo/bar",
					Children: []string{"baz/"},
				},
				{
					Endpoint: "/foo/bar/baz",
					Children: []string{"qux"},
				},
			},
		},
		{
			Name:      "MultipleDifferentiatedEndpoints",
			Endpoints: []string{"/foo/bar", "/baz/qux"},
			Routes: []staticroute.Route{
				{
					Endpoint: "",
					Children: []string{"baz/", "foo/"},
				},
				{
					Endpoint: "/baz",
					Children: []string{"qux"},
				},
				{
					Endpoint: "/foo",
					Children: []string{"bar"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			builder := staticroute.NewBuilder()
			for _, ep := range tc.Endpoints {
				builder.FromEndpoint(ep)
			}

			routes := builder.Build()

			if !cmp.Equal(tc.Routes, routes) {
				t.Fatalf("Unexpected routes: %s", cmp.Diff(tc.Routes, routes))
			}
		})
	}
}
