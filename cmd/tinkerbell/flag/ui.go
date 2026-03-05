package flag

import (
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/ui"
)

// UIConfig holds the flag configuration for the UI service.
type UIConfig struct {
	Config   *ui.Config
	LogLevel int
}

// RegisterUIFlags registers UI service flags with the flag set.
func RegisterUIFlags(fs *Set, h *UIConfig) {
	fs.Register(UIDebugMode, ffval.NewValueDefault(&h.Config.DebugMode, h.Config.DebugMode))
	fs.Register(UILogLevel, ffval.NewValueDefault(&h.LogLevel, h.LogLevel))
	fs.Register(UIURLPrefix, ffval.NewValueDefault(&h.Config.URLPrefix, h.Config.URLPrefix))
	fs.Register(UIEnableAutoLogin, ffval.NewValueDefault(&h.Config.EnableAutoLogin, h.Config.EnableAutoLogin))
}

var UIDebugMode = Config{
	Name:  "ui-debug-mode",
	Usage: "whether to run UI in debug mode",
}

var UILogLevel = Config{
	Name:  "ui-log-level",
	Usage: "the higher the number the more verbose, level 0 inherits the global log level, a negative number disables logging",
}

var UIURLPrefix = Config{
	Name:  "ui-url-prefix",
	Usage: "URL path prefix for the UI",
}

var UIEnableAutoLogin = Config{
	Name:  "ui-enable-auto-login",
	Usage: "use the backend kubeconfig for UI authentication, bypassing the login page",
}

// Convert converts UIConfig data types to ui.Config data types.
func (u *UIConfig) Convert(bindAddr netip.Addr, tlsCertFile, tlsKeyFile string) {
	// Use BindAddr if specified, otherwise use the default
	if bindAddr.IsValid() {
		u.Config.BindAddr = bindAddr.String()
	}

	u.Config.TLSCertFile = tlsCertFile
	u.Config.TLSKeyFile = tlsKeyFile
}
