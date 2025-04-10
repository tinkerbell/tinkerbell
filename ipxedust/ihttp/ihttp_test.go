package ihttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/ipxedust/binary"
	"go.opentelemetry.io/otel/trace"
)

type fakeResponse struct {
	headers http.Header
	body    []byte
	status  int
}

func newFakeResponse() *fakeResponse {
	return &fakeResponse{
		headers: make(http.Header),
	}
}

func (r *fakeResponse) Header() http.Header {
	return r.headers
}

func (r *fakeResponse) Write(body []byte) (int, error) {
	r.body = body
	return len(body), fmt.Errorf("fake error")
}

func (r *fakeResponse) WriteHeader(status int) {
	r.status = status
}

func (r *fakeResponse) Result() *http.Response {
	return &http.Response{
		StatusCode: r.status,
		Body:       ioutil.NopCloser(bytes.NewBuffer(r.body)),
	}
}

func TestListenAndServeHTTP(t *testing.T) {
	router := http.NewServeMux()
	s := Handler{Log: logr.Discard()}
	router.HandleFunc("/", s.Handle)
	srv := &http.Server{Handler: router}
	type args struct {
		ctx  context.Context
		addr netip.AddrPort
		h    *http.Server
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "fail",
			args: args{
				ctx:  context.Background(),
				addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 80),
				h:    &http.Server{},
			},
			wantErr: fmt.Errorf("listen tcp 127.0.0.1:80: bind: permission denied"),
		},
		{
			name: "success",
			args: args{
				ctx:  context.Background(),
				addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 9999),
				h:    srv,
			},
			wantErr: fmt.Errorf("http: Server closed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.ctx.Done()
			tt.args.h.Shutdown(tt.args.ctx)
			err := ListenAndServe(tt.args.ctx, tt.args.addr, tt.args.h)
			if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}

func TestHandle(t *testing.T) {
	patched, _ := binary.Patch(binary.Files["snp.efi"], []byte("echo 'hello world'"))

	type req struct {
		method string
		url    string
		body   io.Reader
	}
	tests := []struct {
		name      string
		req       req
		want      *http.Response
		patch     []byte
		failWrite bool
	}{
		{
			name: "fail",
			req:  req{method: "POST", url: "/fail"},
			want: &http.Response{
				StatusCode: http.StatusMethodNotAllowed,
			},
		},
		{
			name: "success",
			req:  req{method: "GET", url: "/30:23:03:73:a5:a7/snp.efi-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01"},
			want: &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewBuffer(binary.Files["snp.efi"])),
			},
		},
		{
			name: "success - head request",
			req:  req{method: "HEAD", url: "/30:23:03:73:a5:a7/snp.efi-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01"},
			want: &http.Response{
				StatusCode: http.StatusOK,
				Body:       nil,
			},
		},
		{
			name: "fail with bad traceparent",
			req:  req{method: "GET", url: "/30:23:03:73:a5:a7/snp.efi-00-00000000000000000000000000000000-d887dc3912240434-01"},
			want: &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       nil,
			},
		},
		{
			name: "file not found",
			req:  req{method: "GET", url: "/none.efi"},
			want: &http.Response{
				StatusCode: http.StatusNotFound,
			},
		},
		{
			name: "patch",
			req:  req{method: "GET", url: "/30:23:03:73:a5:a7/snp.efi-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01"},
			want: &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewBuffer(patched)),
			},
			patch: []byte("echo 'hello world'"),
		},
		{
			name: "bad patch",
			req:  req{method: "GET", url: "/30:23:03:73:a5:a7/snp.efi-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01"},
			want: &http.Response{
				StatusCode: http.StatusInternalServerError,
			},
			patch: make([]byte, 132),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := stdr.New(log.New(os.Stdout, "", log.Lshortfile))
			req := httptest.NewRequest(tt.req.method, tt.req.url, tt.req.body)
			var resp *http.Response
			if tt.failWrite {
				w := newFakeResponse()
				h := Handler{Log: logger, Patch: tt.patch}
				h.Handle(w, req)
				resp = w.Result()
			} else {
				w := httptest.NewRecorder()
				h := Handler{Log: logger, Patch: tt.patch}
				h.Handle(w, req)
				resp = w.Result()
			}

			defer resp.Body.Close()
			if diff := cmp.Diff(resp.StatusCode, tt.want.StatusCode); diff != "" {
				t.Fatalf(diff)
			}
			if tt.want.Body != nil {
				got, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				want, err := ioutil.ReadAll(tt.want.Body)
				if err != nil {
					t.Fatal(err)
				}

				if diff := cmp.Diff(got, want); diff != "" {
					t.Fatalf(diff)
				}
			}
		})
	}
}

func TestExtractTraceparentFromFilename(t *testing.T) {
	tests := map[string]struct {
		fileIn  string
		fileOut string
		err     error
		spanID  string
		traceID string
	}{
		"do nothing when no tp": {fileIn: "undionly.ipxe", fileOut: "undionly.ipxe", err: nil},
		"ignore bad filename": {
			fileIn:  "undionly.ipxe-00-0000-0000-00",
			fileOut: "undionly.ipxe-00-0000-0000-00",
			err:     nil,
		},
		"ignore corrupt tp": {
			fileIn:  "undionly.ipxe-00-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-abcdefghijklmnop-01",
			fileOut: "undionly.ipxe-00-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-abcdefghijklmnop-01",
			err:     nil,
		},
		"ignore corrupt TraceID": {
			fileIn:  "undionly.ipxe-00-00000000000000000000000000000000-0000000000000000-01",
			fileOut: "undionly.ipxe-00-00000000000000000000000000000000-0000000000000000-01",
			err:     fmt.Errorf("parsing OpenTelemetry trace id %q failed: %w", "00000000000000000000000000000000", fmt.Errorf("trace-id can't be all zero")),
		},
		"ignore corrupt SpanID": {
			fileIn:  "undionly.ipxe-00-11111111111111111111111111111111-0000000000000000-01",
			fileOut: "undionly.ipxe-00-11111111111111111111111111111111-0000000000000000-01",
			err:     fmt.Errorf("parsing OpenTelemetry span id %q failed: %w", "0000000000000000", fmt.Errorf("span-id can't be all zero")),
		},
		"extract tp": {
			fileIn:  "undionly.ipxe-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01",
			fileOut: "undionly.ipxe",
			err:     nil,
			spanID:  "d887dc3912240434",
			traceID: "23b1e307bb35484f535a1f772c06910e",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ctx, outfile, err := extractTraceparentFromFilename(ctx, tc.fileIn)
			if !errors.Is(err, tc.err) {
				if diff := cmp.Diff(fmt.Sprint(err), fmt.Sprint(tc.err)); diff != "" {
					t.Errorf(diff)
					t.Errorf("filename %q should have resulted in error %q but got %q", tc.fileIn, tc.err, err)
				}
			}
			if outfile != tc.fileOut {
				t.Errorf("filename %q should have resulted in %q but got %q", tc.fileIn, tc.fileOut, outfile)
			}

			if tc.spanID != "" {
				sc := trace.SpanContextFromContext(ctx)
				got := sc.SpanID().String()
				if tc.spanID != got {
					t.Errorf("got incorrect span id from context, expected %q but got %q", tc.spanID, got)
				}
			}

			if tc.traceID != "" {
				sc := trace.SpanContextFromContext(ctx)
				got := sc.TraceID().String()
				if tc.traceID != got {
					t.Errorf("got incorrect trace id from context, expected %q but got %q", tc.traceID, got)
				}
			}
		})
	}
}
