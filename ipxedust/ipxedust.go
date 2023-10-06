// Package ipxedust implements the iPXE tftp and http serving.
package ipxedust

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"time"

	"dario.cat/mergo"
	"github.com/go-logr/logr"
	"github.com/pin/tftp/v3"
	"github.com/tinkerbell/ipxedust/ihttp"
	"github.com/tinkerbell/ipxedust/itftp"
	"golang.org/x/sync/errgroup"
)

// Server holds the details for configuring the iPXE service.
type Server struct {
	// TFTP holds the details specific for the TFTP server.
	TFTP ServerSpec
	// HTTP holds the details specific for the HTTP server.
	HTTP ServerSpec
	// Log is the logger to use.
	Log logr.Logger
	// EnableTFTPSinglePort is a flag to enable single port mode for the TFTP server.
	// A standard TFTP server implementation receives requests on port 69 and
	// allocates a new high port (over 1024) dedicated to that request. In single
	// port mode, the same port is used for transmit and receive. If the server
	// is started on port 69, all communication will be done on port 69.
	// This option is required when running in a container that doesn't bind to the hosts
	// network because this type of dynamic port allocation is not generally supported.
	//
	// This option is specific to github.com/pin/tftp. The pin/tftp library says this option is
	// experimental and "Enabling this will negatively impact performance". Please take this into
	// consideration when using this option.
	EnableTFTPSinglePort bool
}

// ServerSpec holds details used to configure a server.
type ServerSpec struct {
	// Addr is the address:port to listen on for requests.
	Addr netip.AddrPort
	// Timeout is the timeout for serving individual requests.
	Timeout time.Duration
	// Disabled allows a server to be disabled. Useful, for example, to disable TFTP.
	Disabled bool
	// BlockSize allows setting a larger maximum block size for TFTP
	BlockSize int
	// The patch to apply to the iPXE binary.
	Patch []byte
}

var errNilListener = fmt.Errorf("listener must not be nil")

// ListenAndServe will listen and serve iPXE binaries over TFTP and HTTP.
//
// Default TFTP listen address is ":69".
//
// Default TFTP block size is 512.
//
// Default HTTP listen address is ":8080".
//
// Default request timeout for both is 5 seconds.
//
// Override the defaults by setting the Config struct fields.
// See binary/binary.go for the iPXE files that are served.
func (c *Server) ListenAndServe(ctx context.Context) error {
	defaults := Server{
		TFTP: ServerSpec{Addr: netip.AddrPortFrom(netip.IPv4Unspecified(), 69), Timeout: 5 * time.Second, BlockSize: 512},
		HTTP: ServerSpec{Addr: netip.AddrPortFrom(netip.IPv4Unspecified(), 8080), Timeout: 5 * time.Second},
		Log:  logr.Discard(),
	}

	err := mergo.Merge(c, defaults, mergo.WithTransformers(c))
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)
	if !c.TFTP.Disabled {
		g.Go(func() error {
			return c.listenAndServeTFTP(ctx)
		})
	}
	if !c.HTTP.Disabled {
		g.Go(func() error {
			return c.listenAndServeHTTP(ctx)
		})
	}

	<-ctx.Done()
	err = g.Wait()
	c.Log.Info("shutting down iPXE servers")

	return err
}

// Serve iPXE binaries over TFTP using udpConn and HTTP using tcpConn.
func (c *Server) Serve(ctx context.Context, tcpConn net.Listener, udpConn net.PacketConn) error {
	if tcpConn == nil {
		return errors.New("tcp listener must not be nil")
	}
	if udpConn == nil {
		return errors.New("udp conn must not be nil")
	}
	defaults := Server{
		TFTP: ServerSpec{Timeout: 5 * time.Second},
		HTTP: ServerSpec{Timeout: 5 * time.Second},
		Log:  logr.Discard(),
	}

	err := mergo.Merge(c, defaults, mergo.WithTransformers(c))
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)
	if !c.TFTP.Disabled {
		g.Go(func() error {
			return c.serveTFTP(ctx, udpConn)
		})
	}
	if !c.HTTP.Disabled {
		g.Go(func() error {
			return c.serveHTTP(ctx, tcpConn)
		})
	}

	<-ctx.Done()
	err = g.Wait()
	c.Log.Info("shutting down iPXE servers")

	return err
}

func (c *Server) listenAndServeHTTP(ctx context.Context) error {
	s := ihttp.Handler{Log: c.Log, Patch: c.HTTP.Patch}
	router := http.NewServeMux()
	router.HandleFunc("/", s.Handle)
	hs := &http.Server{
		Handler:     router,
		BaseContext: func(net.Listener) context.Context { return ctx },
		ReadTimeout: c.HTTP.Timeout,
	}
	c.Log.Info("serving iPXE binaries via HTTP", "addr", c.HTTP.Addr.String(), "timeout", c.HTTP.Timeout)

	go func() {
		<-ctx.Done()
		_ = hs.Shutdown(ctx)
	}()
	err := ihttp.ListenAndServe(ctx, c.HTTP.Addr, hs)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

func (c *Server) serveHTTP(ctx context.Context, l net.Listener) error {
	if l == nil || reflect.ValueOf(l).IsNil() {
		return errNilListener
	}
	s := ihttp.Handler{Log: c.Log, Patch: c.HTTP.Patch}
	router := http.NewServeMux()
	router.HandleFunc("/", s.Handle)
	hs := &http.Server{
		Handler:     router,
		BaseContext: func(net.Listener) context.Context { return ctx },
		ReadTimeout: c.HTTP.Timeout,
	}
	c.Log.Info("serving iPXE binaries via HTTP", "addr", l.Addr().String(), "timeout", c.HTTP.Timeout)
	go func() {
		<-ctx.Done()
		_ = hs.Shutdown(ctx)
	}()

	return ihttp.Serve(ctx, l, hs)
}

func (c *Server) listenAndServeTFTP(ctx context.Context) error {
	a, err := net.ResolveUDPAddr("udp", c.TFTP.Addr.String())
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", a)
	if err != nil {
		return err
	}

	h := &itftp.Handler{Log: c.Log, Patch: c.TFTP.Patch}
	ts := tftp.NewServer(h.HandleRead, h.HandleWrite)
	ts.SetTimeout(c.TFTP.Timeout)
	ts.SetBlockSize(c.TFTP.BlockSize)
	if c.EnableTFTPSinglePort {
		ts.EnableSinglePort()
	}
	c.Log.Info("serving iPXE binaries via TFTP", "addr", c.TFTP.Addr, "blocksize", c.TFTP.BlockSize, "timeout", c.TFTP.Timeout, "singlePortEnabled", c.EnableTFTPSinglePort)
	go func() {
		<-ctx.Done()
		conn.Close()
		ts.Shutdown()
	}()
	return itftp.Serve(ctx, conn, ts)
}

func (c *Server) serveTFTP(ctx context.Context, conn net.PacketConn) error {
	if conn == nil || reflect.ValueOf(conn).IsNil() {
		return errors.New("conn must not be nil")
	}

	h := &itftp.Handler{Log: c.Log, Patch: c.TFTP.Patch}
	ts := tftp.NewServer(h.HandleRead, h.HandleWrite)
	ts.SetTimeout(c.TFTP.Timeout)
	ts.SetBlockSize(c.TFTP.BlockSize)
	if c.EnableTFTPSinglePort {
		ts.EnableSinglePort()
	}
	c.Log.Info("serving iPXE binaries via TFTP", "addr", conn.LocalAddr().String(), "blocksize", c.TFTP.BlockSize, "timeout", c.TFTP.Timeout, "singlePortEnabled", c.EnableTFTPSinglePort)
	go func() {
		<-ctx.Done()
		conn.Close()
		ts.Shutdown()
	}()

	return itftp.Serve(ctx, conn, ts)
}

// Transformer for merging the netip.IPPort and logr.Logger structs.
func (c *Server) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	switch typ {
	case reflect.TypeOf(logr.Logger{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("GetSink")
				result := isZero.Call(nil)
				if result[0].IsNil() {
					dst.Set(src)
				}
			}
			return nil
		}
	case reflect.TypeOf(netip.AddrPort{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				v, ok := dst.Interface().(netip.AddrPort)
				if ok && (v == netip.AddrPort{}) {
					dst.Set(src)
				}
			}
			return nil
		}
	}
	return nil
}
