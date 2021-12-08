// Package ipxe implements the iPXE tftp and http serving.
package ipxe

import (
	"context"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/imdario/mergo"
	"inet.af/netaddr"
)

// Config holds the details for running the iPXE service.
type Config struct {
	// TFTP holds the details for the TFTP server.
	TFTP Server
	// HTTP holds the details for the HTTP server.
	HTTP Server
	// Log is the logger to use.
	Log logr.Logger
}

// Server holds details about a server.
type Server struct {
	// Addr is the address:port to listen on for TFTP requests.
	Addr netaddr.IPPort
	// Timeout is the timeout for serving TFTP files.
	Timeout time.Duration
}

// Serve will listen and serve iPXE binaries over TFTP and HTTP.
// See binary/binary.go for the iPXE files that are served.
func (c *Config) Serve(ctx context.Context) error {
	defaults := Config{
		TFTP: Server{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 69), Timeout: 5 * time.Second},
		HTTP: Server{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 8080), Timeout: 5 * time.Second},
		Log:  logr.Discard(),
	}

	err := mergo.Merge(c, defaults, mergo.WithTransformers(c))
	if err != nil {
		return err
	}

	return nil
}

// Transformer for merging netaddr.IPPort and logr.Logger structs.
func (c *Config) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	switch typ {
	case reflect.TypeOf(logr.Logger{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("GetSink")
				result := isZero.Call([]reflect.Value{})
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
