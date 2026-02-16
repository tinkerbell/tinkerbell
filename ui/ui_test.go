package ui

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewConfig_Defaults(t *testing.T) {
	cfg := NewConfig()
	if cfg.URLPrefix != DefaultURLPrefix {
		t.Errorf("URLPrefix = %q, want %q", cfg.URLPrefix, DefaultURLPrefix)
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
