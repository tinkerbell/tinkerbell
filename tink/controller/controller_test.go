package controller

import "testing"

func TestCacheOptionsNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		want      string
	}{
		{name: "cluster-wide by default"},
		{name: "configured namespace", namespace: "hardware", want: "hardware"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewConfig(WithNamespace(tt.namespace)).cacheOptions()
			if tt.want == "" {
				if len(got.DefaultNamespaces) != 0 {
					t.Fatalf("DefaultNamespaces = %v, want no namespace restriction", got.DefaultNamespaces)
				}
				return
			}
			if _, ok := got.DefaultNamespaces[tt.want]; !ok || len(got.DefaultNamespaces) != 1 {
				t.Fatalf("DefaultNamespaces = %v, want only %q", got.DefaultNamespaces, tt.want)
			}
		})
	}
}
