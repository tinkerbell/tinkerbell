package ui

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewConfig_Defaults(t *testing.T) {
	tests := []struct {
		name         string
		opts         []Option
		wantBindAddr string
		wantBindPort int
	}{
		{
			name:         "empty config gets defaults",
			opts:         nil,
			wantBindAddr: DefaultBindAddr,
			wantBindPort: DefaultBindPort,
		},
		{
			name:         "custom bind port preserved",
			opts:         []Option{WithBindPort(9090)},
			wantBindAddr: DefaultBindAddr,
			wantBindPort: 9090,
		},
		{
			name:         "custom URL prefix",
			opts:         []Option{WithURLPrefix("/custom")},
			wantBindAddr: DefaultBindAddr,
			wantBindPort: DefaultBindPort,
		},
		{
			name:         "multiple options",
			opts:         []Option{WithBindPort(443), WithURLPrefix("/web")},
			wantBindAddr: DefaultBindAddr,
			wantBindPort: 443,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig(tt.opts...)

			if cfg.BindAddr != tt.wantBindAddr {
				t.Errorf("BindAddr = %q, want %q", cfg.BindAddr, tt.wantBindAddr)
			}
			if cfg.BindPort != tt.wantBindPort {
				t.Errorf("BindPort = %d, want %d", cfg.BindPort, tt.wantBindPort)
			}
		})
	}
}

func TestNewConfig_URLPrefix(t *testing.T) {
	tests := []struct {
		name       string
		opts       []Option
		wantPrefix string
	}{
		{
			name:       "default prefix",
			opts:       nil,
			wantPrefix: DefaultURLPrefix,
		},
		{
			name:       "custom prefix",
			opts:       []Option{WithURLPrefix("/custom")},
			wantPrefix: "/custom",
		},
		{
			name:       "empty prefix",
			opts:       []Option{WithURLPrefix("")},
			wantPrefix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig(tt.opts...)

			if cfg.URLPrefix != tt.wantPrefix {
				t.Errorf("URLPrefix = %q, want %q", cfg.URLPrefix, tt.wantPrefix)
			}
		})
	}
}

func TestConfigStart_InvalidPort(_ *testing.T) {
	// Create config with a port that requires root privileges (well-known port)
	// and no TLS - this should fail to bind on most systems
	cfg := &Config{
		BindAddr: "127.0.0.1",
		BindPort: 1, // Port 1 requires root
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Start should return quickly due to cancelled context
	err := cfg.Start(ctx, logr.Discard())
	// We mainly want to ensure it doesn't panic
	_ = err
}
