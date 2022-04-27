package grpcserver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/packethost/cacher/protos/cacher"
	"github.com/packethost/pkg/grpc"
	"github.com/packethost/pkg/log"
	assert "github.com/stretchr/testify/require"
	"github.com/tinkerbell/hegel/datamodel"
	"github.com/tinkerbell/hegel/grpc/protos/hegel"
	"github.com/tinkerbell/hegel/hardware"
	ggrpc "google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

type zapT struct {
	*testing.T
	mu   sync.RWMutex
	done bool
}

// Logs the given message without failing the test.
func (zt *zapT) Logf(format string, args ...interface{}) {
	zt.mu.RLock()
	if !zt.done {
		zt.T.Logf(format, args...)
	}
	zt.mu.RUnlock()
}

// Logs the given message and marks the test as failed.
func (zt *zapT) Errorf(format string, args ...interface{}) {
	zt.mu.RLock()
	if !zt.done {
		zt.T.Logf(format, args...)
	}
	zt.mu.RUnlock()
}

// startServersAndConnectClient starts 2 grpc services.
// First is a fake upstream cacher (fakeServer) that just sends back the provided interface and error.
// Second is an instance of hegel that uses fakeServer as its upstream.
// A hegel client connected to the second server is returned.
func startServersAndConnectClient(t *testing.T, d map[string]string, err error) (context.Context, context.CancelFunc, cacher.CacherClient, hegel.HegelClient) {
	t.Helper()

	ctx, cancelCtx := context.WithCancel(context.Background())
	startServerAndConnectClient := func(name string, server *grpc.Server) ggrpc.ClientConnInterface {
		t.Helper()

		listener := bufconn.Listen(bufSize)
		go func() {
			t.Helper()

			if err := server.Server().Serve(listener); err != nil {
				t.Error(fmt.Errorf("%s.Serve exited with error: %w", name, err))
			}
		}()

		dialer := func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}

		conn, err := ggrpc.DialContext(ctx, "bufnet", ggrpc.WithContextDialer(dialer), ggrpc.WithInsecure())
		if err != nil {
			t.Fatalf("Failed to dial %s: %v", name, err)
		}

		return conn
	}

	zt := &zapT{
		T: t,
	}
	cancel := func() {
		cancelCtx()
		zt.mu.Lock()
		zt.done = true
		zt.mu.Unlock()
	}

	// This is a fake Cacher
	name := "fakeServer"
	fakeServer, err := grpc.NewServer(log.Test(zt, name), func(s *grpc.Server) {
		cacher.RegisterCacherServer(s.Server(), &fakeServer{
			data:     d,
			err:      err,
			watchers: make(map[string]chan string),
		})
	})
	assert.NoError(t, err)
	cClient := cacher.NewCacherClient(startServerAndConnectClient(name, fakeServer))
	assert.NotNil(t, cClient)

	// This is a Hegel
	name = "hegel"
	l := log.Test(zt, name)
	hg, err := hardware.NewCacherClient(cClient, datamodel.Cacher)
	assert.NoError(t, err)
	hegelServer := NewServer(l, hg)

	server, err := grpc.NewServer(l, func(s *grpc.Server) {
		hegel.RegisterHegelServer(s.Server(), hegelServer)
	})
	assert.NoError(t, err)
	client := hegel.NewHegelClient(startServerAndConnectClient(name, server))
	assert.NotNil(t, client)

	return ctx, cancel, cClient, client
}

func assertGRPCError(t *testing.T, errWanted, errGot error) {
	t.Helper()

	if errWanted != nil {
		assert.NotNil(t, errGot)
		tStatus, ok := status.FromError(errWanted)
		assert.True(t, ok)
		eStatus, ok := status.FromError(errGot)
		assert.True(t, ok)
		assert.Equal(t, tStatus.Code(), eStatus.Code())
		assert.Equal(t, tStatus.Message(), eStatus.Message())
	} else {
		assert.Nil(t, errGot)
	}
}

type fakeServer struct {
	err      error
	mu       sync.Mutex
	data     map[string]string
	watchers map[string]chan string
}

func (s *fakeServer) All(_ *cacher.Empty, stream cacher.Cacher_AllServer) error {
	if s.err != nil {
		return s.err
	}
	for _, v := range s.data {
		stream.Send(&cacher.Hardware{
			JSON: v,
		})
	}
	return s.err
}

func (s *fakeServer) ByID(_ context.Context, r *cacher.GetRequest) (*cacher.Hardware, error) {
	h := &cacher.Hardware{
		JSON: s.data[r.ID],
	}
	return h, s.err
}

func (s *fakeServer) ByIP(_ context.Context, r *cacher.GetRequest) (*cacher.Hardware, error) {
	h := &cacher.Hardware{
		JSON: s.data[r.IP],
	}
	return h, s.err
}

func (s *fakeServer) ByMAC(_ context.Context, r *cacher.GetRequest) (*cacher.Hardware, error) {
	h := &cacher.Hardware{
		JSON: s.data[r.MAC],
	}
	return h, s.err
}

func (s *fakeServer) Ingest(context.Context, *cacher.Empty) (*cacher.Empty, error) {
	return nil, s.err
}

func (s *fakeServer) Push(_ context.Context, r *cacher.PushRequest) (*cacher.Empty, error) {
	if s.err != nil {
		return nil, s.err
	}

	kv := strings.SplitN(r.Data, "=", 2)
	k := kv[0]
	v := kv[0]
	if len(kv) > 1 {
		v = kv[1]
	}

	var watcher chan string
	s.mu.Lock()
	s.data[k] = v
	watcher = s.watchers[k]
	s.mu.Unlock()

	if watcher != nil {
		watcher <- v
	}

	return &cacher.Empty{}, s.err
}

func (s *fakeServer) Watch(r *cacher.GetRequest, stream cacher.Cacher_WatchServer) error {
	if s.err != nil {
		return s.err
	}

	ch := make(chan string, 5)
	s.mu.Lock()
	s.watchers[r.ID] = ch
	s.mu.Unlock()

loop:
	for {
		select {
		case <-stream.Context().Done():
			s.mu.Lock()
			delete(s.watchers, r.ID)
			s.mu.Unlock()

			close(ch)
			break loop
		case hw := <-ch:
			stream.Send(&cacher.Hardware{
				JSON: hw,
			})
		}
	}
	return s.err
}
