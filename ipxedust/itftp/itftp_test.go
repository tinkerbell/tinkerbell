package itftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/pin/tftp/v3"
	"github.com/tinkerbell/ipxedust/binary"
	"go.opentelemetry.io/otel/trace"
)

type fakeReaderFrom struct {
	addr    net.UDPAddr
	content []byte
	err     error
}

func (f *fakeReaderFrom) ReadFrom(r io.Reader) (n int64, err error) {
	if f.err != nil {
		return 0, f.err
	}
	nInt, err := r.Read(f.content)
	return int64(nInt), err
}

func (f *fakeReaderFrom) SetSize(_ int64) {}

func (f *fakeReaderFrom) RemoteAddr() net.UDPAddr {
	return f.addr
}

func (f *fakeReaderFrom) WriteTo(_ io.Writer) (n int64, err error) {
	return 0, nil
}

func TestListenAndServeTFTP(t *testing.T) {
	ht := &Handler{Log: logr.Discard()}
	srv := tftp.NewServer(ht.HandleRead, ht.HandleWrite)
	type args struct {
		ctx  context.Context
		addr netip.AddrPort
		h    *tftp.Server
	}
	tests := []struct {
		name    string
		args    args
		wantErr interface{}
	}{
		{
			name: "fail",
			args: args{
				ctx:  context.Background(),
				addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 80),
				h:    nil,
			},
			wantErr: &net.OpError{},
		},
		{
			name: "success",
			args: args{
				ctx:  context.Background(),
				addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 9999),
				h:    srv,
			},
			wantErr: interface{}(nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errChan := make(chan error, 1)
			go func() {
				errChan <- ListenAndServe(tt.args.ctx, tt.args.addr, tt.args.h)
			}()

			if tt.args.h != nil {
				time.Sleep(time.Second)
				tt.args.h.Shutdown()
			}
			err := <-errChan
			if !errors.As(err, &tt.wantErr) && err != nil {
				t.Fatalf("error mismatch, got: %T, want: %T", err, tt.wantErr)
			}
		})
	}
}

func TestHandleRead(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		patch    []byte
		want     []byte
		wantErr  error
	}{
		{
			name:     "success",
			fileName: "snp.efi",
			want:     binary.Files["snp.efi"],
		},
		{
			name:     "success - with otel name",
			fileName: "snp.efi-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01",
			want:     binary.Files["snp.efi"],
		},
		{
			name:     "fail with bad traceparent",
			fileName: "snp.efi-00-00000000000000000000000000000000-d887dc3912240434-01",
			wantErr:  os.ErrNotExist,
		},
		{
			name:     "fail - not found",
			fileName: "not-found",
			wantErr:  os.ErrNotExist,
		},
		{
			name:     "failure - with read error",
			fileName: "snp.efi",
			wantErr:  net.ErrClosed,
		},
		{
			name:     "failure - bad patch",
			fileName: "snp.efi",
			patch:    make([]byte, 500),
			wantErr:  binary.ErrPatchTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ht := &Handler{Log: logr.Discard(), Patch: tt.patch}
			rf := &fakeReaderFrom{
				addr:    net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9999},
				content: make([]byte, len(tt.want)),
				err:     tt.wantErr,
			}
			err := ht.HandleRead(tt.fileName, rf)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error mismatch, got: %T, want: %T", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				tt.want = []byte{}
			}
			if diff := cmp.Diff(rf.content, tt.want); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestHandleWrite(t *testing.T) {
	ht := &Handler{Log: logr.Discard()}
	rf := &fakeReaderFrom{addr: net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9999}}
	err := ht.HandleWrite("snp.efi", rf)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("error mismatch, got: %T, want: %T", err, os.ErrPermission)
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
