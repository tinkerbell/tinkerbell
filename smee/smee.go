package smee

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/tinkerbell/ipxedust"
	"github.com/tinkerbell/ipxedust/ihttp"
	"github.com/tinkerbell/tinkerbell/data"
	"github.com/tinkerbell/tinkerbell/otel"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp/handler/proxy"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp/handler/reservation"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp/server"
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

const (
	DHCPModeProxy       DHCPMode = "proxy"
	DHCPModeReservation DHCPMode = "reservation"
	DHCPModeAutoProxy   DHCPMode = "auto-proxy"
	// isoMagicString comes from the HookOS repo and is used to patch the HookOS ISO image.
	// ref: https://github.com/tinkerbell/hook/blob/main/linuxkit-templates/hook.template.yaml
	isoMagicString = `464vn90e7rbj08xbwdjejmdf4it17c5zfzjyfhthbh19eij201hjgit021bmpdb9ctrc87x2ymc8e7icu4ffi15x1hah9iyaiz38ckyap8hwx2vt5rm44ixv4hau8iw718q5yd019um5dt2xpqqa2rjtdypzr5v1gun8un110hhwp8cex7pqrh2ivh0ynpm4zkkwc8wcn367zyethzy7q8hzudyeyzx3cgmxqbkh825gcak7kxzjbgjajwizryv7ec1xm2h0hh7pz29qmvtgfjj1vphpgq1zcbiiehv52wrjy9yq473d9t1rvryy6929nk435hfx55du3ih05kn5tju3vijreru1p6knc988d4gfdz28eragvryq5x8aibe5trxd0t6t7jwxkde34v6pj1khmp50k6qqj3nzgcfzabtgqkmeqhdedbvwf3byfdma4nkv3rcxugaj2d0ru30pa2fqadjqrtjnv8bu52xzxv7irbhyvygygxu1nt5z4fh9w1vwbdcmagep26d298zknykf2e88kumt59ab7nq79d8amnhhvbexgh48e8qc61vq2e9qkihzt1twk1ijfgw70nwizai15iqyted2dt9gfmf2gg7amzufre79hwqkddc1cd935ywacnkrnak6r7xzcz7zbmq3kt04u2hg1iuupid8rt4nyrju51e6uejb2ruu36g9aibmz3hnmvazptu8x5tyxk820g2cdpxjdij766bt2n3djur7v623a2v44juyfgz80ekgfb9hkibpxh3zgknw8a34t4jifhf116x15cei9hwch0fye3xyq0acuym8uhitu5evc4rag3ui0fny3qg4kju7zkfyy8hwh537urd5uixkzwu5bdvafz4jmv7imypj543xg5em8jk8cgk7c4504xdd5e4e71ihaumt6u5u2t1w7um92fepzae8p0vq93wdrd1756npu1pziiur1payc7kmdwyxg3hj5n4phxbc29x0tcddamjrwt260b0w`
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

type Config struct {
	Backend    BackendReader
	Logger     logr.Logger
	DHCP       DHCP
	IPXE       IPXE
	ISO        ISO
	OTEL       OTEL
	Syslog     Syslog
	TFTP       TFTP
	TinkServer TinkServer
}

type Syslog struct {
	BindAddr netip.Addr
	BindPort uint16
	Enabled  bool
}

type TFTP struct {
	BindAddr  netip.Addr
	BindPort  uint16
	BlockSize int
	Timeout   time.Duration
	Enabled   bool
}

type IPXE struct {
	EmbeddedScriptPatch string
	HTTPBinaryServer    IPXEHTTPBinaryServer
	HTTPScriptServer    IPXEHTTPScriptServer
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
	Enabled           bool
	Mode              DHCPMode
	BindAddr          netip.AddrPort
	BindInterface     string
	IPForPacket       netip.Addr
	SyslogIP          netip.Addr
	TFTPIP            netip.Addr
	IPXEHTTPBinaryURL *url.URL
	IPXEHTTPScript    IPXEHTTPScript
	TFTPPort          uint16
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

// Start will run Smee services. Enabling and disabling services is controlled by the Config struct.
func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	c.Logger = log
	if c.Backend == nil {
		c.Backend = noop{}
		c.Logger.Info("no backend provided, using noop backend")
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
		addr := fmt.Sprintf("%s:%d", c.Syslog.BindAddr, c.Syslog.BindPort)
		log.Info("starting syslog server", "bind_addr", addr)
		g.Go(func() error {
			if err := syslog.StartReceiver(ctx, log, addr, 1); err != nil {
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
		tftpServer := &ipxedust.Server{
			Log:                  log.WithValues("service", "github.com/tinkerbell/smee").WithName("github.com/tinkerbell/ipxedust"),
			HTTP:                 ipxedust.ServerSpec{Disabled: true}, // disabled because below we use the http handlerfunc instead.
			EnableTFTPSinglePort: true,
		}
		tftpServer.EnableTFTPSinglePort = true
		addrPort := netip.AddrPortFrom(c.TFTP.BindAddr, c.TFTP.BindPort)
		tftpServer.TFTP = ipxedust.ServerSpec{
			Disabled:  false,
			Addr:      addrPort,
			Timeout:   c.TFTP.Timeout,
			Patch:     []byte(c.IPXE.EmbeddedScriptPatch),
			BlockSize: c.TFTP.BlockSize,
		}
		// start the ipxe binary tftp server
		log.Info("starting tftp server", "bind_addr", addrPort.String())
		g.Go(func() error {
			return tftpServer.ListenAndServe(ctx)
		})
	}

	handlers := http.HandlerMapping{}
	// http ipxe binaries
	if c.IPXE.HTTPBinaryServer.Enabled {
		// 1. data validation
		// 2. start the http server for ipxe binaries
		// serve ipxe binaries from the "/ipxe/" URI.
		handlers["/ipxe/"] = ihttp.Handler{
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
		log.Info("serving http", "addr", bindAddr.String(), "trusted_proxies", c.IPXE.HTTPScriptServer.TrustedProxies)
		g.Go(func() error {
			return httpServer.ServeHTTP(ctx, bindAddr.String(), handlers)
		})
	}

	// dhcp serving
	if c.DHCP.Enabled {
		dh, err := c.dhcpHandler()
		if err != nil {
			return fmt.Errorf("failed to create dhcp listener: %w", err)
		}
		log.Info("starting dhcp server", "bind_addr", c.DHCP.BindAddr)
		g.Go(func() error {
			conn, err := server4.NewIPv4UDPConn(c.DHCP.BindInterface, net.UDPAddrFromAddrPort(c.DHCP.BindAddr))
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
	log.Info("smee is shutting down", "reason", ctx.Err())
	return nil
}

func (c *Config) dhcpHandler() (server.Handler, error) {
	// 1. create the handler
	// 2. create the backend
	// 3. add the backend to the handler
	tftpIP := netip.AddrPortFrom(c.DHCP.TFTPIP, c.DHCP.TFTPPort)

	httpBinaryURL := *c.DHCP.IPXEHTTPBinaryURL

	httpScriptURL := c.DHCP.IPXEHTTPScript.URL

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
			Log:     c.Logger,
			Netboot: reservation.Netboot{
				IPXEBinServerTFTP: tftpIP,
				IPXEBinServerHTTP: &httpBinaryURL,
				IPXEScriptURL:     ipxeScript,
				Enabled:           true,
			},
			OTELEnabled: true,
			SyslogAddr:  c.DHCP.SyslogIP,
		}
		return dh, nil
	case DHCPModeProxy:
		dh := &proxy.Handler{
			Backend: c.Backend,
			IPAddr:  c.DHCP.IPForPacket,
			Log:     c.Logger,
			Netboot: proxy.Netboot{
				IPXEBinServerTFTP: tftpIP,
				IPXEBinServerHTTP: &httpBinaryURL,
				IPXEScriptURL:     ipxeScript,
				Enabled:           true,
			},
			OTELEnabled:      true,
			AutoProxyEnabled: false,
		}
		return dh, nil
	case DHCPModeAutoProxy:
		dh := &proxy.Handler{
			Backend: c.Backend,
			IPAddr:  c.DHCP.IPForPacket,
			Log:     c.Logger,
			Netboot: proxy.Netboot{
				IPXEBinServerTFTP: tftpIP,
				IPXEBinServerHTTP: &httpBinaryURL,
				IPXEScriptURL:     ipxeScript,
				Enabled:           true,
			},
			OTELEnabled:      true,
			AutoProxyEnabled: true,
		}
		return dh, nil
	}

	return nil, errors.New("invalid dhcp mode")
}
