package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/boots-ipxe/binary"
	"inet.af/netaddr"
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
	s := HandleHTTP{Log: logr.Discard()}
	router.HandleFunc("/", s.Handler)
	srv := &http.Server{Handler: router}
	type args struct {
		ctx  context.Context
		addr netaddr.IPPort
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
				addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 80),
				h:    &http.Server{},
			},
			wantErr: fmt.Errorf("listen tcp 127.0.0.1:80: bind: permission denied"),
		},
		{
			name: "success",
			args: args{
				ctx:  context.Background(),
				addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 9999),
				h:    srv,
			},
			wantErr: fmt.Errorf("http: Server closed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.ctx.Done()
			tt.args.h.Shutdown(tt.args.ctx)
			err := ListenAndServeHTTP(tt.args.ctx, tt.args.addr, tt.args.h)
			if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}

func TestHandleHTTP_Handler(t *testing.T) {
	type req struct {
		method string
		url    string
		body   io.Reader
	}
	tests := []struct {
		name      string
		req       req
		want      *http.Response
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
			req:  req{method: "GET", url: "/snp.efi"},
			want: &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewBuffer(binary.Files["snp.efi"])),
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
			name: "write failure",
			req:  req{method: "GET", url: "/snp.efi"},
			want: &http.Response{
				StatusCode: http.StatusInternalServerError,
			},
			failWrite: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.req.method, tt.req.url, tt.req.body)
			var resp *http.Response
			if tt.failWrite {
				w := newFakeResponse()
				h := HandleHTTP{Log: logr.Discard()}
				h.Handler(w, req)
				resp = w.Result()
			} else {
				w := httptest.NewRecorder()
				h := HandleHTTP{Log: logr.Discard()}
				h.Handler(w, req)
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
