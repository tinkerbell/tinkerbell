package kube

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"k8s.io/client-go/rest/fake"
)

func TestWaitForAPIServer(t *testing.T) {
	tests := map[string]struct {
		response     *http.Response
		maxWaitTime  time.Duration
		pollInterval time.Duration
		expectError  bool
	}{
		"API server becomes healthy": {
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       newRepeatableReader([]byte("ok")),
			},
			maxWaitTime:  1 * time.Second,
			pollInterval: 50 * time.Millisecond,
			expectError:  false,
		},
		"maxWaitTime is reached": {
			response: &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       newRepeatableReader([]byte("")),
			},
			maxWaitTime:  10 * time.Millisecond,
			pollInterval: 2 * time.Millisecond,
			expectError:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fr := &fake.RESTClient{
				Resp: tc.response,
			}

			b := &Backend{}
			err := b.WaitForAPIServer(context.Background(), logr.Discard(), tc.maxWaitTime, tc.pollInterval, fr)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if diff := cmp.Diff(err.Error(), "timed out waiting for API server to be healthy and ready after 10ms"); diff != "" {
					t.Errorf("expected timeout error, got %v", diff)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestEndpointReady(t *testing.T) {
	tests := map[string]struct {
		expectError bool
		response    *http.Response
		errorMsg    string
	}{
		"API server is ready": {
			expectError: false,
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("ok")),
			},
		},
		"API server is not ready": {
			expectError: true,
			response: &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString("")),
			},
			errorMsg: "API server not ready yet, error: the server could not find the requested resource",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fr := &fake.RESTClient{
				Resp: tc.response,
			}
			err := checkEndpoint(context.Background(), endpointLivez, "ok", fr)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if diff := cmp.Diff(err.Error(), tc.errorMsg); diff != "" {
					t.Errorf("expected API server not ready error, got %v", diff)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

func newRepeatableReader(data []byte) io.ReadCloser {
	return &repeatReader{
		data:      bytes.NewReader(data),
		readCount: new(uint32),
	}
}

type repeatReader struct {
	data      *bytes.Reader
	readCount *uint32
}

func (r *repeatReader) Read(p []byte) (int, error) {
	r.data.Seek(0, io.SeekStart)
	n, err := r.data.Read(p)

	// Increment read count atomically
	count := atomic.AddUint32(r.readCount, 1)

	// Return EOF only on even reads. This is to signal to the caller that the data has been read completely.
	if err == nil || count%2 == 0 {
		return n, io.EOF
	}

	return n, err
}

func (r *repeatReader) Close() error {
	return nil
}
