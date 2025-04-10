//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/tinkerbell/hegel/internal/cmd"
)

func TestHegel(t *testing.T) {
	// Build the root command so we can launch it as if a main() func would.
	root, err := cmd.NewRootCommand()
	if err != nil {
		t.Fatal(err)
	}

	root.SetArgs([]string{
		// Use the flatfile backend to limit our dependencies. We're really focused on ensuring
		// the root cmd strings together properly so this should be fine.
		"--backend", "flatfile",
		"--flatfile-path", "testdata/e2e.yml",

		// We need to trust the localhost so we can impersonate machines in requests.
		"--trusted-proxies", "127.0.0.1",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go root.ExecuteContext(ctx)

	// Ensure the cmd goroutine is scheduled (by leaning on continuation behavior of the runtime)
	// and begins listening. Slower machines may need a longer delay.
	time.Sleep(50 * time.Millisecond)

	t.Run("EC2", func(t *testing.T) {
		// We have unit tests to validate the APIs serve correct data. These tests are to validate
		// a static endpoint and a dynamic endpoint work as expected.
		cases := []struct {
			Name     string
			Endpoint string
			Expect   string
		}{
			{
				Name:     "StaticRoute",
				Endpoint: "/2009-04-04",
				Expect: `meta-data/
user-data`,
			},
			{
				Name:     "DynamicRoute",
				Endpoint: "/2009-04-04/meta-data/hostname",
				Expect:   "hostname",
			},
		}

		for _, tc := range cases {
			t.Run(tc.Name, func(t *testing.T) {
				request, err := http.NewRequest(
					http.MethodGet,
					"http://127.0.0.1:50061"+tc.Endpoint,
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}

				// Impersonate the target instance.
				request.Header.Add("X-Forwarded-For", "10.10.10.10")

				response, err := http.DefaultClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()

				// Store the body in a buffer for comparison.
				var buf bytes.Buffer
				_, err = io.Copy(&buf, response.Body)
				if err != nil {
					t.Fatal(err)
				}

				if buf.String() != tc.Expect {
					t.Fatalf("Expected:\n%s\n\nReceived:\n%s\n", tc.Expect, buf.String())
				}
			})
		}
	})
}
