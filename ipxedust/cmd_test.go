package ipxedust

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/phayes/freeport"
)

func TestCommand_RegisterFlags(t *testing.T) {
	tests := []struct {
		name string
		want *flag.FlagSet
	}{
		{"success", func() *flag.FlagSet {
			c := &Command{}
			fs := flag.NewFlagSet("ipxe", flag.ExitOnError)
			fs.StringVar(&c.TFTPAddr, "tftp-addr", "0.0.0.0:69", "TFTP server address")
			fs.IntVar(&c.TFTPBlockSize, "tftp-blocksize", 512, "TFTP server maximum block size")
			fs.DurationVar(&c.TFTPTimeout, "tftp-timeout", time.Second*5, "TFTP server timeout")
			fs.StringVar(&c.HTTPAddr, "http-addr", "0.0.0.0:8080", "HTTP server address")
			fs.DurationVar(&c.HTTPTimeout, "http-timeout", time.Second*5, "HTTP server timeout")
			fs.StringVar(&c.LogLevel, "log-level", "info", "Log level")
			fs.BoolVar(&c.EnableTFTPSinglePort, "tftp-single-port", false, "Enable single port mode for TFTP server (needed for container deploys)")
			return fs
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Command{}
			fs := flag.NewFlagSet("ipxe", flag.ExitOnError)
			c.RegisterFlags(fs)
			if diff := cmp.Diff(fs, tt.want, cmp.AllowUnexported(flag.FlagSet{}), cmpopts.IgnoreFields(flag.FlagSet{}, "Usage")); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func getPort() int {
	port, _ := freeport.GetFreePort()
	return port
}

func TestCommand_Run(t *testing.T) {
	tests := []struct {
		name    string
		cmd     *Command
		wantErr error
	}{
		{"success", &Command{TFTPAddr: fmt.Sprintf("0.0.0.0:%v", getPort()), HTTPAddr: fmt.Sprintf("0.0.0.0:%v", getPort())}, nil},
		{"fail permission denied", &Command{TFTPAddr: "127.0.0.1:80"}, fmt.Errorf("listen udp 127.0.0.1:80: bind: permission denied")},
		{"fail parse error", &Command{TFTPAddr: "127.0.0.1:AF"}, fmt.Errorf(`invalid port "AF" parsing "127.0.0.1:AF"`)},
		{"fail parse error", &Command{HTTPAddr: "127.0.0.1:AF"}, fmt.Errorf(`invalid port "AF" parsing "127.0.0.1:AF"`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.cmd
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			errChan := make(chan error, 1)
			go func() {
				errChan <- c.Run(ctx)
			}()
			<-ctx.Done()
			got := <-errChan
			if errors.Is(got, context.DeadlineExceeded) {
				got = nil
			}
			if diff := cmp.Diff(fmt.Sprint(got), fmt.Sprint(tt.wantErr)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestCommand_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cmd     *Command
		wantErr error
	}{
		{"success", &Command{
			TFTPAddr:      "0.0.0.0:69",
			TFTPBlockSize: 512,
			TFTPTimeout:   5 * time.Second,
			HTTPAddr:      "0.0.0.0:8080",
			HTTPTimeout:   5 * time.Second,
			Log:           logr.Discard(),
			LogLevel:      "info",
		}, nil},
		{"fail", &Command{
			TFTPBlockSize: 512,
			TFTPTimeout:   5 * time.Second,
			HTTPAddr:      "0.0.0.0:8080",
			HTTPTimeout:   5 * time.Second,
			Log:           logr.Discard(),
			LogLevel:      "info",
		}, fmt.Errorf(`Key: 'Command.TFTPAddr' Error:Field validation for 'TFTPAddr' failed on the 'required' tag`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.cmd
			got := c.Validate()
			if diff := cmp.Diff(fmt.Sprint(got), fmt.Sprint(tt.wantErr)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr error
	}{
		{"success", []string{fmt.Sprintf("--tftp-addr=0.0.0.0:%v", getPort()), fmt.Sprintf("--http-addr=0.0.0.0:%v", getPort())}, nil},
		{"fail validation", []string{"--tftp-addr=0.0.0.0:AF", "--log-level=debug"}, fmt.Errorf(`Key: 'Command.TFTPAddr' Error:Field validation for 'TFTPAddr' failed on the 'hostname_port' tag`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			errChan := make(chan error, 1)
			go func() {
				errChan <- Execute(ctx, tt.args)
			}()
			<-ctx.Done()
			got := <-errChan
			if errors.Is(got, context.DeadlineExceeded) {
				got = nil
			}
			if diff := cmp.Diff(fmt.Sprint(got), fmt.Sprint(tt.wantErr)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
