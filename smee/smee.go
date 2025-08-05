package smee

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path"
	"reflect"
	"strings"
	"time"

	"dario.cat/mergo"
	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/otel"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp/handler/proxy"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp/handler/reservation"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp/server"
	"github.com/tinkerbell/tinkerbell/smee/internal/ipxe/binary"
	"github.com/tinkerbell/tinkerbell/smee/internal/ipxe/http"
	"github.com/tinkerbell/tinkerbell/smee/internal/ipxe/script"
	"github.com/tinkerbell/tinkerbell/smee/internal/iso"
	"github.com/tinkerbell/tinkerbell/smee/internal/metric"
	"github.com/tinkerbell/tinkerbell/smee/internal/syslog"
	"golang.org/x/sync/errgroup"
)

// BackendReader is the interface for getting data from a backend.
type BackendReader interface {
	// Read data (from a backend) based on a mac address
	// and return DHCP headers and options, including netboot info.
	GetByMac(context.Context, net.HardwareAddr) (*data.DHCP, *data.Netboot, error)
	GetByIP(context.Context, net.IP) (*data.DHCP, *data.Netboot, error)
}

type MacAddrFormat string

func (m MacAddrFormat) String() string {
	return string(m)
}

const (
	DHCPModeProxy       DHCPMode = "proxy"
	DHCPModeReservation DHCPMode = "reservation"
	DHCPModeAutoProxy   DHCPMode = "auto-proxy"
	// isoMagicString comes from the HookOS repo and is used to patch the HookOS ISO image.
	// ref: https://github.com/tinkerbell/hook/blob/main/linuxkit-templates/hook.template.yaml
	isoMagicString = `464vn90e7rbj08xbwdjejmdf4it17c5zfzjyfhthbh19eij201hjgit021bmpdb9ctrc87x2ymc8e7icu4ffi15x1hah9iyaiz38ckyap8hwx2vt5rm44ixv4hau8iw718q5yd019um5dt2xpqqa2rjtdypzr5v1gun8un110hhwp8cex7pqrh2ivh0ynpm4zkkwc8wcn367zyethzy7q8hzudyeyzx3cgmxqbkh825gcak7kxzjbgjajwizryv7ec1xm2h0hh7pz29qmvtgfjj1vphpgq1zcbiiehv52wrjy9yq473d9t1rvryy6929nk435hfx55du3ih05kn5tju3vijreru1p6knc988d4gfdz28eragvryq5x8aibe5trxd0t6t7jwxkde34v6pj1khmp50k6qqj3nzgcfzabtgqkmeqhdedbvwf3byfdma4nkv3rcxugaj2d0ru30pa2fqadjqrtjnv8bu52xzxv7irbhyvygygxu1nt5z4fh9w1vwbdcmagep26d298zknykf2e88kumt59ab7nq79d8amnhhvbexgh48e8qc61vq2e9qkihzt1twk1ijfgw70nwizai15iqyted2dt9gfmf2gg7amzufre79hwqkddc1cd935ywacnkrnak6r7xzcz7zbmq3kt04u2hg1iuupid8rt4nyrju51e6uejb2ruu36g9aibmz3hnmvazptu8x5tyxk820g2cdpxjdij766bt2n3djur7v623a2v44juyfgz80ekgfb9hkibpxh3zgknw8a34t4jifhf116x15cei9hwch0fye3xyq0acuym8uhitu5evc4rag3ui0fny3qg4kju7zkfyy8hwh537urd5uixkzwu5bdvafz4jmv7imypj543xg5em8jk8cgk7c4504xdd5e4e71ihaumt6u5u2t1w7um92fepzae8p0vq93wdrd1756npu1pziiur1payc7kmdwyxg3hj5n4phxbc29x0tcddamjrwt260b0w`

	// Defaults consumers can use.
	DefaultTFFTPPort      = 69
	DefaultTFFTPBlockSize = 512
	DefaultTFFTPTimeout   = 10 * time.Second

	DefaultDHCPMode = DHCPModeReservation

	DefaultSyslogPort = 514

	DefaultHTTPPort = 7171

	DefaultTinkServerPort = 42113

	MacAddrFormatColon MacAddrFormat = "colon"
	MacAddrFormatDot   MacAddrFormat = "dot"
	MacAddrFormatDash  MacAddrFormat = "dash"
	MacAddrFormatNone  MacAddrFormat = "none"
)

type DHCPMode string

func (d DHCPMode) String() string {
	return string(d)
}

func (d *DHCPMode) Set(s string) error {
	switch strings.ToLower(s) {
	case string(DHCPModeProxy), string(DHCPModeReservation), string(DHCPModeAutoProxy):
		*d = DHCPMode(s)
		return nil
	default:
		return fmt.Errorf("invalid DHCP mode: %q, must be one of [%s, %s, %s]", s, DHCPModeReservation, DHCPModeProxy, DHCPModeAutoProxy)
	}
}

func (d *DHCPMode) Type() string {
	return "dhcp-mode"
}

// Config is the configuration for the Smee service.
type Config struct {
	// Backend is the backend to use for getting data.
	Backend BackendReader
	// DHCP is the configuration for the DHCP service.
	DHCP DHCP
	// IPXE is the configuration for the iPXE service.
	IPXE IPXE
	// ISO is the configuration for the ISO service.
	ISO ISO
	// OTEL is the configuration for OpenTelemetry.
	OTEL OTEL
	// Syslog is the configuration for the syslog service.
	Syslog Syslog
	// TFTP is the configuration for the TFTP service.
	TFTP TFTP
	// TinkServer is the configuration for the Tinkerbell server.
	TinkServer TinkServer
}

type Syslog struct {
	// BindAddr is the local address to which to bind the syslog server.
	BindAddr netip.Addr
	// BindPort is the local port to which to bind the syslog server.
	BindPort uint16
	// Enabled is a flag to enable or disable the syslog server.
	Enabled bool
}

type TFTP struct {
	// BindAddr is the local address to which to bind the TFTP server.
	BindAddr netip.Addr
	// BindPort is the local port to which to bind the TFTP server.
	BindPort uint16
	// BlockSize is the block size to use when serving TFTP requests.
	BlockSize int
	// Timeout is the timeout for each serving each TFTP request.
	Timeout time.Duration
	// Enabled is a flag to enable or disable the TFTP server.
	Enabled bool
}

type IPXE struct {
	EmbeddedScriptPatch string
	HTTPBinaryServer    IPXEHTTPBinaryServer
	HTTPScriptServer    IPXEHTTPScriptServer

	// IPXEBinary are the options to use when serving iPXE binaries via TFTP or HTTP.
	IPXEBinary IPXEHTTPBinary
}

type IPXEHTTPBinaryServer struct {
	Enabled bool
}

type IPXEHTTPScriptServer struct {
	Enabled         bool
	BindAddr        netip.Addr
	BindPort        uint16
	Retries         int
	RetryDelay      int
	OSIEURL         *url.URL
	TrustedProxies  []string
	ExtraKernelArgs []string
}

type DHCP struct {
	// Enabled configures whether the DHCP server is enabled.
	Enabled bool
	// EnableNetbootOptions configures whether sending netboot options is enabled.
	EnableNetbootOptions bool
	// Mode determines the behavior of the DHCP server.
	// See the DHCPMode type for valid values.
	Mode DHCPMode
	// BindAddr is the local address to which to bind the DHCP server and listen for DHCP packets.
	BindAddr netip.Addr
	BindPort uint16
	// BindInterface is the local interface to which to bind the DHCP server and listen for DHCP packets.
	BindInterface string
	// IPForPacket is the IP address to use in the DHCP packet for DHCP option 54.
	IPForPacket netip.Addr
	// SyslogIP is the IP address to use in the DHCP packet for DHCP option 7.
	SyslogIP netip.Addr
	// TFTPIP is the IP address to use in the DHCP packet for DHCP option 66.
	TFTPIP netip.Addr
	// TFTPPort is the port to use in the DHCP packet for DHCP option 66.
	TFTPPort uint16
	// IPXEHTTPBinaryURL is the URL to the iPXE binary server serving via HTTP.
	IPXEHTTPBinaryURL *url.URL
	// IPXEHTTPScript is the URL to the iPXE script to use.
	IPXEHTTPScript IPXEHTTPScript
}

type IPXEHTTPBinary struct {
	// InjectMacAddrFormat is the format to use when injecting the mac address into the iPXE binary URL.
	// Valid values are "colon", "dot", "dash", and "none".
	// For example, colon: http://1.2.3.4/ipxe/ipxe.efi -> http://1.2.3.4/ipxe/40:15:ff:89:cc:0e/ipxe.efi
	InjectMacAddrFormat MacAddrFormat
}

type IPXEHTTPScript struct {
	URL *url.URL
	// InjectMacAddress will prepend the hardware mac address to the ipxe script URL file name.
	// For example: http://1.2.3.4/my/loc/auto.ipxe -> http://1.2.3.4/my/loc/40:15:ff:89:cc:0e/auto.ipxe
	// Setting this to false is useful when you are not using the auto.ipxe script in Smee.
	InjectMacAddress bool
}

type OTEL struct {
	Endpoint         string
	InsecureEndpoint bool
}

type ISO struct {
	Enabled           bool
	UpstreamURL       *url.URL
	PatchMagicString  string
	StaticIPAMEnabled bool
}

type TinkServer struct {
	UseTLS      bool
	InsecureTLS bool
	AddrPort    string
}

// NewConfig is a constructor for the Config struct. It will set default values for the Config struct.
// Boolean fields are not set-able via c. To set boolean, modify the returned Config struct.
func NewConfig(c Config, publicIP netip.Addr) *Config {
	defaults := &Config{
		DHCP: DHCP{
			Enabled:              true,
			EnableNetbootOptions: true,
			Mode:                 DefaultDHCPMode,
			BindAddr:             netip.MustParseAddr("0.0.0.0"),
			BindPort:             67,
			BindInterface:        "",
			IPXEHTTPBinaryURL: &url.URL{
				Scheme: "http",
				Path:   "/ipxe",
			},
			IPXEHTTPScript: IPXEHTTPScript{
				URL: &url.URL{
					Scheme: "http",
					Path:   "auto.ipxe",
				},
				InjectMacAddress: true,
			},
			TFTPPort: DefaultTFFTPPort,
		},
		IPXE: IPXE{
			EmbeddedScriptPatch: "",
			HTTPBinaryServer: IPXEHTTPBinaryServer{
				Enabled: true,
			},
			HTTPScriptServer: IPXEHTTPScriptServer{
				Enabled:         true,
				BindAddr:        publicIP,
				BindPort:        DefaultHTTPPort,
				Retries:         1,
				RetryDelay:      1,
				OSIEURL:         &url.URL{},
				TrustedProxies:  []string{},
				ExtraKernelArgs: []string{},
			},
			IPXEBinary: IPXEHTTPBinary{
				InjectMacAddrFormat: MacAddrFormatColon,
			},
		},
		ISO: ISO{
			Enabled:           false,
			UpstreamURL:       &url.URL{},
			PatchMagicString:  "",
			StaticIPAMEnabled: false,
		},
		OTEL: OTEL{
			Endpoint:         "",
			InsecureEndpoint: false,
		},
		Syslog: Syslog{
			BindAddr: publicIP,
			BindPort: DefaultSyslogPort,
			Enabled:  true,
		},
		TFTP: TFTP{
			BindAddr:  publicIP,
			BindPort:  DefaultTFFTPPort,
			BlockSize: DefaultTFFTPBlockSize,
			Timeout:   DefaultTFFTPTimeout,
			Enabled:   true,
		},
		TinkServer: TinkServer{},
	}

	if err := mergo.Merge(defaults, &c, mergo.WithTransformers(&c)); err != nil {
		panic(fmt.Sprintf("failed to merge config: %v", err))
	}

	return defaults
}

// Start will run Smee services. Enabling and disabling services is controlled by the Config struct.
func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	if c.Backend == nil {
		return errors.New("no backend provided")
	}
	oCfg := otel.Config{
		Servicename: "smee",
		Endpoint:    c.OTEL.Endpoint,
		Insecure:    c.OTEL.InsecureEndpoint,
		Logger:      log,
	}
	ctx, otelShutdown, err := otel.Init(ctx, oCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
	}
	defer otelShutdown()
	metric.Init()

	g, ctx := errgroup.WithContext(ctx)
	// syslog
	if c.Syslog.Enabled {
		// 1. data validation
		// 2. start the syslog server
		addr := netip.AddrPortFrom(c.Syslog.BindAddr, c.Syslog.BindPort)
		if !addr.IsValid() {
			return fmt.Errorf("invalid syslog bind address: IP: %v, Port: %v", addr.Addr(), addr.Port())
		}
		log.Info("starting syslog server", "bindAddr", addr)
		g.Go(func() error {
			if err := syslog.StartReceiver(ctx, log, addr.String(), 1); err != nil {
				log.Error(err, "syslog server failure")
				return err
			}
			<-ctx.Done()
			log.Info("syslog server stopped")
			return nil
		})
	}

	// tftp
	if c.TFTP.Enabled {
		// 1. data validation
		// 2. start the tftp server
		addrPort := netip.AddrPortFrom(c.TFTP.BindAddr, c.TFTP.BindPort)
		if !addrPort.IsValid() {
			return fmt.Errorf("invalid TFTP bind address: IP: %v, Port: %v", addrPort.Addr(), addrPort.Port())
		}
		tftpHandler := binary.TFTP{
			Log:                  log.WithValues("service", "github.com/tinkerbell/smee").WithName("github.com/tinkerbell/ipxedust"),
			EnableTFTPSinglePort: true,
			Addr:                 addrPort,
			Timeout:              c.TFTP.Timeout,
			Patch:                []byte(c.IPXE.EmbeddedScriptPatch),
			BlockSize:            c.TFTP.BlockSize,
		}

		// start the ipxe binary tftp server
		log.Info("starting tftp server", "bindAddr", addrPort.String())
		g.Go(func() error {
			return tftpHandler.ListenAndServe(ctx)
		})
	}

	handlers := http.HandlerMapping{}
	// http ipxe binaries
	if c.IPXE.HTTPBinaryServer.Enabled {
		// 1. data validation
		// 2. start the http server for ipxe binaries
		// serve ipxe binaries from the "/ipxe/" URI.
		handlers["/ipxe/"] = binary.Handler{
			Log:   log.WithValues("service", "github.com/tinkerbell/smee").WithName("github.com/tinkerbell/ipxedust"),
			Patch: []byte(c.IPXE.EmbeddedScriptPatch),
		}.Handle
	}

	// http ipxe script
	if c.IPXE.HTTPScriptServer.Enabled {
		jh := script.Handler{
			Logger:                log,
			Backend:               c.Backend,
			OSIEURL:               c.IPXE.HTTPScriptServer.OSIEURL.String(),
			ExtraKernelParams:     c.IPXE.HTTPScriptServer.ExtraKernelArgs,
			PublicSyslogFQDN:      c.DHCP.SyslogIP.String(),
			TinkServerTLS:         c.TinkServer.UseTLS,
			TinkServerInsecureTLS: c.TinkServer.InsecureTLS,
			TinkServerGRPCAddr:    c.TinkServer.AddrPort,
			IPXEScriptRetries:     c.IPXE.HTTPScriptServer.Retries,
			IPXEScriptRetryDelay:  c.IPXE.HTTPScriptServer.RetryDelay,
			StaticIPXEEnabled:     (c.DHCP.Mode == DHCPModeAutoProxy),
		}

		// serve ipxe script from the "/" URI.
		handlers["/"] = jh.HandlerFunc()
	}

	if c.ISO.Enabled {
		// 1. data validation
		// 2. start the http server for iso images
		ih := iso.Handler{
			Logger:             log,
			Backend:            c.Backend,
			SourceISO:          c.ISO.UpstreamURL.String(),
			ExtraKernelParams:  c.IPXE.HTTPScriptServer.ExtraKernelArgs,
			Syslog:             c.DHCP.SyslogIP.String(),
			TinkServerTLS:      c.TinkServer.UseTLS,
			TinkServerGRPCAddr: c.TinkServer.AddrPort,
			StaticIPAMEnabled:  c.ISO.StaticIPAMEnabled,
			MagicString: func() string {
				if c.ISO.PatchMagicString == "" {
					return isoMagicString
				}
				return c.ISO.PatchMagicString
			}(),
		}
		isoHandler, err := ih.HandlerFunc()
		if err != nil {
			return fmt.Errorf("failed to create iso handler: %w", err)
		}
		handlers["/iso/"] = isoHandler
	}

	if len(handlers) > 0 {
		// 1. data validation
		// 2. start the http server for ipxe binaries and scripts
		// start the http server for ipxe binaries and scripts

		httpServer := &http.Config{
			GitRev:         "",
			StartTime:      time.Now(),
			Logger:         log,
			TrustedProxies: c.IPXE.HTTPScriptServer.TrustedProxies,
		}
		bindAddr := netip.AddrPortFrom(c.IPXE.HTTPScriptServer.BindAddr, c.IPXE.HTTPScriptServer.BindPort)
		if !bindAddr.IsValid() {
			return fmt.Errorf("invalid HTTP Script Server bind address: IP: %v, Port: %v", bindAddr.Addr(), bindAddr.Port())
		}
		log.Info("starting http server", "addr", bindAddr.String(), "trustedProxies", c.IPXE.HTTPScriptServer.TrustedProxies)
		g.Go(func() error {
			return httpServer.ServeHTTP(ctx, bindAddr.String(), handlers)
		})
	}

	// dhcp serving
	if c.DHCP.Enabled {
		dh, err := c.dhcpHandler(log)
		if err != nil {
			return fmt.Errorf("failed to create dhcp listener: %w", err)
		}
		dhcpAddrPort := netip.AddrPortFrom(c.DHCP.BindAddr, c.DHCP.BindPort)
		if !dhcpAddrPort.IsValid() {
			return fmt.Errorf("invalid DHCP bind address: IP: %v, Port: %v", dhcpAddrPort.Addr(), dhcpAddrPort.Port())
		}
		log.Info("starting dhcp server", "bindAddr", dhcpAddrPort)
		g.Go(func() error {
			conn, err := server4.NewIPv4UDPConn(c.DHCP.BindInterface, net.UDPAddrFromAddrPort(dhcpAddrPort))
			if err != nil {
				return err
			}
			defer conn.Close()
			ds := &server.DHCP{Logger: log, Conn: conn, Handlers: []server.Handler{dh}}

			return ds.Serve(ctx)
		})
	}

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("failed running all Smee services: %w", err)
	}
	if c.noServicesEnabled() {
		return errors.New("no services enabled")
	}
	log.Info("smee is shutting down", "reason", ctx.Err())
	return nil
}

func (c *Config) dhcpHandler(log logr.Logger) (server.Handler, error) {
	// 1. create the handler
	// 2. create the backend
	// 3. add the backend to the handler
	tftpIP := netip.AddrPortFrom(c.DHCP.TFTPIP, c.DHCP.TFTPPort)
	if !tftpIP.IsValid() {
		return nil, fmt.Errorf("invalid TFTP bind address: IP: %v, Port: %v", tftpIP.Addr(), tftpIP.Port())
	}

	httpBinaryURL := *c.DHCP.IPXEHTTPBinaryURL

	httpScriptURL := c.DHCP.IPXEHTTPScript.URL

	if httpScriptURL == nil {
		return nil, errors.New("http ipxe script url is required")
	}
	if _, err := url.Parse(httpScriptURL.String()); err != nil {
		return nil, fmt.Errorf("invalid http ipxe script url: %w", err)
	}
	ipxeScript := func(*dhcpv4.DHCPv4) *url.URL {
		return httpScriptURL
	}
	if c.DHCP.IPXEHTTPScript.InjectMacAddress {
		ipxeScript = func(d *dhcpv4.DHCPv4) *url.URL {
			u := *httpScriptURL
			p := path.Base(u.Path)
			u.Path = path.Join(path.Dir(u.Path), d.ClientHWAddr.String(), p)
			return &u
		}
	}

	switch c.DHCP.Mode {
	case DHCPModeReservation:
		dh := &reservation.Handler{
			Backend: c.Backend,
			IPAddr:  c.DHCP.IPForPacket,
			Log:     log,
			Netboot: reservation.Netboot{
				IPXEBinServerTFTP:   tftpIP,
				IPXEBinServerHTTP:   &httpBinaryURL,
				IPXEScriptURL:       ipxeScript,
				Enabled:             c.DHCP.EnableNetbootOptions,
				InjectMacAddrFormat: dhcp.MacAddrFormat(c.IPXE.IPXEBinary.InjectMacAddrFormat),
			},
			OTELEnabled: true,
			SyslogAddr:  c.DHCP.SyslogIP,
		}
		return dh, nil
	case DHCPModeProxy:
		dh := &proxy.Handler{
			Backend: c.Backend,
			IPAddr:  c.DHCP.IPForPacket,
			Log:     log,
			Netboot: proxy.Netboot{
				IPXEBinServerTFTP:   tftpIP,
				IPXEBinServerHTTP:   &httpBinaryURL,
				IPXEScriptURL:       ipxeScript,
				Enabled:             c.DHCP.EnableNetbootOptions,
				InjectMacAddrFormat: dhcp.MacAddrFormat(c.IPXE.IPXEBinary.InjectMacAddrFormat),
			},
			OTELEnabled:      true,
			AutoProxyEnabled: false,
		}
		return dh, nil
	case DHCPModeAutoProxy:
		dh := &proxy.Handler{
			Backend: c.Backend,
			IPAddr:  c.DHCP.IPForPacket,
			Log:     log,
			Netboot: proxy.Netboot{
				IPXEBinServerTFTP: tftpIP,
				IPXEBinServerHTTP: &httpBinaryURL,
				IPXEScriptURL:     ipxeScript,
				Enabled:           c.DHCP.EnableNetbootOptions,
			},
			OTELEnabled:      true,
			AutoProxyEnabled: true,
		}
		return dh, nil
	}

	return nil, errors.New("invalid dhcp mode")
}

// Transformer for merging the netip.IPPort and logr.Logger structs.
func (c *Config) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	var zeroUint16 uint16
	var zeroInt int
	var zeroDuration time.Duration
	switch typ {
	case reflect.TypeOf(logr.Logger{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := src.MethodByName("GetSink")
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
				v, ok := src.Interface().(netip.AddrPort)
				if ok && (v != netip.AddrPort{}) {
					dst.Set(src)
				}
			}
			return nil
		}
	case reflect.TypeOf(netip.Addr{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				v, ok := src.Interface().(netip.Addr)
				if ok && (v.Compare(netip.Addr{}) != 0) {
					dst.Set(src)
				}
			}
			return nil
		}
	case reflect.TypeOf(zeroUint16):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				v, ok := src.Interface().(uint16)
				if ok && v != 0 {
					dst.Set(src)
				}
			}
			return nil
		}
	case reflect.TypeOf(zeroInt):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				v, ok := src.Interface().(int)
				if ok && v != 0 {
					dst.Set(src)
				}
			}
			return nil
		}
	case reflect.TypeOf(zeroDuration):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				v, ok := src.Interface().(time.Duration)
				if ok && v != 0 {
					dst.Set(src)
				}
			}
			return nil
		}
	}
	return nil
}

func (c *Config) noServicesEnabled() bool {
	return !c.DHCP.Enabled && !c.TFTP.Enabled && !c.ISO.Enabled && !c.Syslog.Enabled && !c.IPXE.HTTPBinaryServer.Enabled && !c.IPXE.HTTPScriptServer.Enabled
}
