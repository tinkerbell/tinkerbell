package backend_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/tinkerbell/hegel/internal/backend"
	"github.com/tinkerbell/hegel/internal/backend/kubernetes"
)

func TestNew(t *testing.T) {
	cases := []struct {
		Name    string
		Options Options
		Error   error
	}{
		{
			Name: "OnlyOneBackend",
			Options: Options{
				Flatfile:   &Flatfile{},
				Kubernetes: &kubernetes.Config{},
			},
			Error: ErrMultipleBackends,
		},
		{
			Name:    "MissingBackend",
			Options: Options{},
			Error:   ErrMissingBackendConfig,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			_, err := New(context.Background(), tc.Options)

			if err == nil {
				t.Fatal("Expected error, received nil")
			}

			if !errors.Is(err, tc.Error) {
				t.Fatalf("Expected: %v;\nReceived: %v", tc.Error, err)
			}
		})
	}
}
