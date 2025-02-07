package http_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	hhttp "github.com/tinkerbell/tinkerbell/hegel/internal/http"
)

// TestServe validates the Serve function does in-fact serve a functional HTTP server with the
// desired handler.
func TestServe(t *testing.T) {
	l := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger := logr.FromSlogHandler(l.Handler())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "Hello, world!")
	})

	go hhttp.Serve(ctx, logger, fmt.Sprintf(":%d", 45555), &mux)

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://localhost:45555")
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
	l := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger := logr.FromSlogHandler(l.Handler())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	n, err := net.Listen("tcp", ":8181")
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	if err := hhttp.Serve(ctx, logger, fmt.Sprintf(":%d", 8181), &http.ServeMux{}); err == nil {
		t.Fatal("expected error")
	}
}
