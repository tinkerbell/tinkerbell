//go:build integration

package http_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
	. "github.com/tinkerbell/hegel/internal/http"
)

// TestServe validates the Serve function does in-fact serve a functional HTTP server with the
// desired handler.
func TestServe(t *testing.T) {
	zl := zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	logger := zerologr.New(&zl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hello, world!")
	})

	go Serve(ctx, logger, fmt.Sprintf(":%d", 8080), &mux)

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatal("expected status code 200")
	}

	var buf bytes.Buffer
	io.Copy(&buf, resp.Body)

	if buf.String() != "Hello, world!" {
		t.Fatal("expected body to be 'Hello, world!'")
	}
}

func TestServerFailure(t *testing.T) {
	zl := zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	logger := zerologr.New(&zl)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	n, err := net.Listen("tcp", ":8181")
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	if err := Serve(ctx, logger, fmt.Sprintf(":%d", 8181), &http.ServeMux{}); err == nil {
		t.Fatal("expected error")
	}
}
