// Package ipxe implements the iPXE tftp and http serving.
package ipxe

import (
	"context"
	"errors"
	"net"
	"net/http"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/imdario/mergo"
	"github.com/pin/tftp"
	ihttp "github.com/tinkerbell/boots-ipxe/http"
	itftp "github.com/tinkerbell/boots-ipxe/tftp"
	"golang.org/x/sync/errgroup"
	"inet.af/netaddr"
)

// Server holds the details for configuring the iPXE service.
type Server struct {
	// TFTP holds the details specific for the TFTP server.
	TFTP ServerSpec
	// HTTP holds the details specific for the HTTP server.
	HTTP ServerSpec
	// Log is the logger to use.
	Log logr.Logger
}

// ServerSpec holds details used to configure a server.
type ServerSpec struct {
	// Addr is the address:port to listen on for requests.
	Addr netaddr.IPPort
	// Timeout is the timeout for serving individual requests.
	Timeout time.Duration
}

// ListenAndServe will listen and serve iPXE binaries over TFTP and HTTP.
//
// Default TFTP listen address is ":69".
//
// Default HTTP listen address is ":8080".
//
// Default request timeout for both is 5 seconds.
//
// Override the defaults by setting the Config struct fields.
// See binary/binary.go for the iPXE files that are served.
func (c *Server) ListenAndServe(ctx context.Context) error {
	defaults := Server{
		TFTP: ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 69), Timeout: 5 * time.Second},
		HTTP: ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 8080), Timeout: 5 * time.Second},
		Log:  logr.Discard(),
	}

	err := mergo.Merge(c, defaults, mergo.WithTransformers(c))
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return c.listenAndServeTFTP(ctx)
	})
	g.Go(func() error {
		return c.listenAndServeHTTP(ctx)
	})

	<-ctx.Done()
	err = g.Wait()
	c.Log.Info("shutting down")

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
	g.Go(func() error {
		return c.serveTFTP(ctx, udpConn)
	})
	g.Go(func() error {
		return c.serveHTTP(ctx, tcpConn)
	})

	<-ctx.Done()
	err = g.Wait()
	c.Log.Info("shutting down")

	return err
}

func (c *Server) listenAndServeHTTP(ctx context.Context) error {
	s := ihttp.Handler{Log: c.Log}
	router := http.NewServeMux()
	router.HandleFunc("/", s.Handle)
	hs := &http.Server{
		Handler:     router,
		BaseContext: func(net.Listener) context.Context { return ctx },
		ReadTimeout: c.HTTP.Timeout,
	}
	c.Log.Info("serving HTTP", "addr", c.HTTP.Addr.String(), "timeout", c.HTTP.Timeout)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return ihttp.ListenAndServe(ctx, c.HTTP.Addr, hs)
	})

	<-ctx.Done()
	err := hs.Shutdown(ctx)
	if err != nil {
		return err
	}
	err = g.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

func (c *Server) serveHTTP(ctx context.Context, l net.Listener) error {
	if l == nil || reflect.ValueOf(l).IsNil() {
		return errors.New("listener must not be nil")
	}
	s := ihttp.Handler{Log: c.Log}
	router := http.NewServeMux()
	router.HandleFunc("/", s.Handle)
	hs := &http.Server{
		Handler:     router,
		BaseContext: func(net.Listener) context.Context { return ctx },
		ReadTimeout: c.HTTP.Timeout,
	}
	c.Log.Info("serving HTTP", "addr", l.Addr().String(), "timeout", c.HTTP.Timeout)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return ihttp.Serve(ctx, l, hs)
	})

	<-ctx.Done()
	err := hs.Shutdown(ctx)
	if err != nil {
		return err
	}
	err = g.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
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

	h := &itftp.Handler{Log: c.Log}
	ts := tftp.NewServer(h.HandleRead, h.HandleWrite)
	ts.SetTimeout(c.TFTP.Timeout)
	c.Log.Info("serving TFTP", "addr", c.TFTP.Addr, "timeout", c.TFTP.Timeout)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return itftp.Serve(ctx, conn, ts)
	})
	// The time.Sleep(time.Second) is load bearing. It allows the tftp server shutdown below to not nil pointer error
	// if a canceled context is passed in to the serveTFTP() function. This happens because itftp.Serve must be called
	// for ts.conn to be populated. ts.Shutdown needs ts.conn to be populated to close the connection or else it panics.
	// One option to "fix" this issue is to PR the following into github.com/pin/tftp:
	/*
			func (s *Server) Shutdown() {
			if !s.singlePort {
				if s.conn != nil {
					s.conn.Close()
				}
			}
			q := make(chan struct{})
			s.quit <- q
			<-q
			s.wg.Wait()
		}
	*/
	time.Sleep(time.Second)
	<-ctx.Done()
	ts.Shutdown()

	return g.Wait()
}

func (c *Server) serveTFTP(ctx context.Context, conn net.PacketConn) error {
	if conn == nil || reflect.ValueOf(conn).IsNil() {
		return errors.New("conn must not be nil")
	}

	h := &itftp.Handler{Log: c.Log}
	ts := tftp.NewServer(h.HandleRead, h.HandleWrite)
	ts.SetTimeout(c.TFTP.Timeout)
	c.Log.Info("serving TFTP", "addr", conn.LocalAddr().String(), "timeout", c.TFTP.Timeout)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return itftp.Serve(ctx, conn, ts)
	})
	// The time.Sleep(time.Second) is load bearing. It allows the tftp server shutdown below to not nil pointer error
	// if a canceled context is passed in to the serveTFTP() function. This happens because itftp.Serve must be called
	// for ts.conn to be populated. ts.Shutdown needs ts.conn to be populated to close the connection or else it panics.
	// One option to "fix" this issue is to PR the following into github.com/pin/tftp:
	/*
			func (s *Server) Shutdown() {
			if !s.singlePort {
				if s.conn != nil {
					s.conn.Close()
				}
			}
			q := make(chan struct{})
			s.quit <- q
			<-q
			s.wg.Wait()
		}
	*/
	time.Sleep(time.Second)
	<-ctx.Done()
	ts.Shutdown()
	return g.Wait()
}

// Transformer for merging the netaddr.IPPort and logr.Logger structs.
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
	case reflect.TypeOf(netaddr.IPPort{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("IsZero")
				result := isZero.Call([]reflect.Value{})
				if result[0].Bool() {
					dst.Set(src)
				}
			}
			return nil
		}
	}
	return nil
}
