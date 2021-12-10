// Package ipxe implements the iPXE tftp and http serving.
package ipxe

import (
	"context"
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

// Config holds the details for running the iPXE service.
type Config struct {
	// TFTP holds the details for the TFTP server.
	TFTP Attribute
	// HTTP holds the details for the HTTP server.
	HTTP Attribute
	// Log is the logger to use.
	Log logr.Logger
}

// Attribute holds details about a server.
type Attribute struct {
	// Addr is the address:port to listen on for requests.
	Addr netaddr.IPPort
	// Timeout is the timeout for serving requests.
	Timeout time.Duration
}

// ListenAndServe will listen and serve iPXE binaries over TFTP and HTTP.
// See binary/binary.go for the iPXE files that are served.
func (c *Config) ListenAndServe(ctx context.Context) error {
	defaults := Config{
		TFTP: Attribute{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 69), Timeout: 5 * time.Second},
		HTTP: Attribute{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 8080), Timeout: 5 * time.Second},
		Log:  logr.Discard(),
	}

	err := mergo.Merge(c, defaults, mergo.WithTransformers(c))
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return c.serveTFTP(ctx)
	})
	g.Go(func() error {
		return c.serveHTTP(ctx)
	})

	<-ctx.Done()
	e := g.Wait()
	c.Log.Info("shutting down")

	return e
}

func (c *Config) serveHTTP(ctx context.Context) error {
	s := ihttp.Handler{Log: c.Log}
	router := http.NewServeMux()
	router.HandleFunc("/", s.Handle)
	hs := &http.Server{Handler: router, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		<-ctx.Done()
		_ = hs.Shutdown(ctx)
	}()
	c.Log.Info("serving HTTP", "addr", c.HTTP.Addr, "timeout", c.HTTP.Timeout)

	return ihttp.ListenAndServe(ctx, c.HTTP.Addr, hs)
}

func (c *Config) serveTFTP(ctx context.Context) error {
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
	// This log line is load bearing. It allows the tftp server shutdown below to not nil pointer error
	// if a canceled context is passed in to the serveTFTP() function.
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
	c.Log.Info("serving TFTP", "addr", c.TFTP.Addr, "timeout", c.TFTP.Timeout)
	go func() {
		<-ctx.Done()
		if conn != nil {
			ts.Shutdown()
		}
	}()

	return itftp.Serve(ctx, conn, ts)
}

// Transformer for merging the netaddr.IPPort and logr.Logger structs.
func (c *Config) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
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
