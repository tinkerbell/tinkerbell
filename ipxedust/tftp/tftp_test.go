package tftp

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/pin/tftp"
	"github.com/tinkerbell/boots-ipxe/binary"
	"go.opentelemetry.io/otel/trace"
	"inet.af/netaddr"
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
	ht := &HandleTFTP{Log: logr.Discard()}
	srv := tftp.NewServer(ht.ReadHandler, ht.WriteHandler)
	type args struct {
		ctx  context.Context
		addr netaddr.IPPort
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
				addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 80),
				h:    nil,
			},
			wantErr: &net.OpError{},
		},
		{
			name: "success",
			args: args{
				ctx:  context.Background(),
				addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 9999),
				h:    srv,
			},
			wantErr: interface{}(nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errChan := make(chan error, 1)
			go func() {
				errChan <- ListenAndServeTFTP(tt.args.ctx, tt.args.addr, tt.args.h)
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

func TestHandlerTFTP_ReadHandler(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
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
			name:     "fail - not found",
			fileName: "not-found",
			want:     []byte{},
			wantErr:  os.ErrNotExist,
		},
		{
			name:     "failure - with read error",
			fileName: "snp.efi",
			want:     []byte{},
			wantErr:  net.ErrClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ht := &HandleTFTP{Log: logr.Discard()}
			rf := &fakeReaderFrom{
				addr:    net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9999},
				content: make([]byte, len(tt.want)),
				err:     tt.wantErr,
			}
			err := ht.ReadHandler(tt.fileName, rf)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error mismatch, got: %T, want: %T", err, tt.wantErr)
			}
			if diff := cmp.Diff(rf.content, tt.want); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestHandlerTFTP_WriteHandler(t *testing.T) {
	ht := &HandleTFTP{Log: logr.Discard()}
	rf := &fakeReaderFrom{addr: net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9999}}
	err := ht.WriteHandler("snp.efi", rf)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("error mismatch, got: %T, want: %T", err, os.ErrPermission)
	}
}

func TestExtractTraceparentFromFilename(t *testing.T) {
	tests := map[string]struct {
		fileIn  string
		fileOut string
		err     error
		spanId  string
		traceId string
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
		"extract tp": {
			fileIn:  "undionly.ipxe-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01",
			fileOut: "undionly.ipxe",
			err:     nil,
			spanId:  "d887dc3912240434",
			traceId: "23b1e307bb35484f535a1f772c06910e",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ctx, outfile, err := extractTraceparentFromFilename(ctx, tc.fileIn)
			if err != tc.err {
				t.Errorf("filename %q should have resulted in error %q but got %q", tc.fileIn, tc.err, err)
			}
			if outfile != tc.fileOut {
				t.Errorf("filename %q should have resulted in %q but got %q", tc.fileIn, tc.fileOut, outfile)
			}

			if tc.spanId != "" {
				sc := trace.SpanContextFromContext(ctx)
				got := sc.SpanID().String()
				if tc.spanId != got {
					t.Errorf("got incorrect span id from context, expected %q but got %q", tc.spanId, got)
				}
			}

			if tc.traceId != "" {
				sc := trace.SpanContextFromContext(ctx)
				got := sc.TraceID().String()
				if tc.traceId != got {
					t.Errorf("got incorrect trace id from context, expected %q but got %q", tc.traceId, got)
				}
			}
		})
	}
}
