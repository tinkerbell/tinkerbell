package iso

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/smee/internal/iso/internal"
)

const (
	defaultConsoles     = "console=ttyAMA0 console=ttyS0 console=tty0 console=tty1 console=ttyS1"
	queryParamSourceISO = "sourceISO"
	schemeHTTP          = "http"
	schemeHTTPS         = "https"
)

// BackendReader is an interface that defines the method to read data from a backend.
type BackendReader interface {
	// Read data (from a backend) based on a mac address
	// and return DHCP headers and options, including netboot info.
	GetByMac(context.Context, net.HardwareAddr) (data.Hardware, error)
}

// Handler is a struct that contains the necessary fields to patch an ISO file with
// relevant information for the Tink worker.
type Handler struct {
	Backend BackendReader
	Logger  logr.Logger
	Patch   Patch
}

// Patch holds the data and configuration used for ISO patching.
type Patch struct {
	KernelParams KernelParams
	// MagicString is the string pattern that will be matched
	// in the source iso before patching. The field can be set
	// during build time by setting this field.
	// Ref: https://github.com/tinkerbell/hook/blob/main/linuxkit-templates/hook.template.yaml
	MagicString     string
	magicStrPadding []byte
	// SourceISO is the source url where the unmodified iso lives.
	// It must be a valid url.URL{} object and must have a url.URL{}.Scheme of HTTP or HTTPS.
	SourceISO         string
	StaticIPAMEnabled bool
}

// KernelParams holds the values used as kernel parameters when patching an ISO.
type KernelParams struct {
	ExtraParams        []string
	Syslog             string
	TinkServerTLS      bool
	TinkServerGRPCAddr string
}

// HandlerFunc returns a reverse proxy HTTP handler function that performs ISO patching.
func (h *Handler) HandlerFunc() (http.HandlerFunc, error) {
	// Parse and validate the default SourceISO.
	defaultSourceISO := &url.URL{}
	if h.Patch.SourceISO != "" {
		t, err := url.Parse(h.Patch.SourceISO)
		if err != nil {
			return nil, err
		}
		if _, err := validateURL(t); err != nil {
			return nil, fmt.Errorf("unsupported scheme in SourceISO: %s (only http and https are supported)", t.Scheme)
		}
		defaultSourceISO = t
	}

	proxy := &internal.ReverseProxy{
		Rewrite: func(pr *internal.ProxyRequest) {
			tu, err := targetURL(pr.In.URL.Query().Get(queryParamSourceISO), "", defaultSourceISO.String())
			if err != nil {
				pr.SetURL(defaultSourceISO)
				h.Logger.Error(err, "error parsing target URL from query parameter, using default SourceISO", "defaultSourceISO", defaultSourceISO.String(), queryParamSourceISO, pr.In.URL.Query().Get(queryParamSourceISO))
				return
			}
			pr.SetURL(tu)
		},
		Transport:     h,
		FlushInterval: -1,
		CopyBuffer:    h,
	}

	h.Patch.magicStrPadding = bytes.Repeat([]byte{' '}, len(h.Patch.MagicString))

	return proxy.ServeHTTP, nil
}

// targetURL returns a valid URL from the first non-empty source and an error, if any.
//
// The order of precedence for sources is:
//
// 1. From query parameter "isoFromQuery"
//
// 2. From hardware object "isoFromHWObject"
//
// 3. From config "isoFromConfig"
func targetURL(isoFromQuery, isoFromHWObject, isoFromConfig string) (*url.URL, error) {
	if isoFromQuery != "" {
		tu, err := url.Parse(isoFromQuery)
		if err != nil {
			return nil, fmt.Errorf("invalid sourceISO URL in query parameter: %w", err)
		}

		return validateURL(tu)
	}

	if isoFromHWObject != "" {
		tu, err := url.Parse(isoFromHWObject)
		if err != nil {
			return nil, fmt.Errorf("invalid sourceISO URL in hardware: %w", err)
		}

		return validateURL(tu)
	}

	if isoFromConfig != "" {
		tu, err := url.Parse(isoFromConfig)
		if err != nil {
			return nil, fmt.Errorf("invalid sourceISO URL in default: %w", err)
		}

		return validateURL(tu)
	}

	return nil, fmt.Errorf("no ISO provided, one from query parameter, hardware object, or config must be set")
}

func validateURL(u *url.URL) (*url.URL, error) {
	if u == nil {
		return nil, errors.New("URL is nil")
	}
	if !slices.Contains([]string{schemeHTTP, schemeHTTPS}, u.Scheme) {
		return nil, fmt.Errorf("unsupported scheme in URL: %s (only http and https are allowed)", u.Scheme)
	}
	return u, nil
}

// Copy implements the internal.CopyBuffer interface.
// This implementation allows us to inspect and patch content on its way to the client without buffering the entire response
// in memory. This allows memory use to be constant regardless of the size of the response.
func (h *Handler) Copy(ctx context.Context, dst io.Writer, src io.Reader, buf []byte) (int64, error) {
	if len(buf) == 0 {
		buf = make([]byte, 32*1024)
	}
	var written int64
	for {
		nr, rerr := src.Read(buf)
		if rerr != nil && rerr != io.EOF && rerr != context.Canceled { //nolint: errorlint // going to defer to the stdlib on this one.
			h.Logger.Info("httputil: ReverseProxy read error during body copy: %v", rerr)
		}
		if nr > 0 {
			// This is the patching check and handling.
			b := buf[:nr]
			i := bytes.Index(b, []byte(h.Patch.MagicString))
			if i != -1 {
				dup := make([]byte, len(b))
				copy(dup, b)
				copy(dup[i:], h.Patch.magicStrPadding)
				copy(dup[i:], internal.GetPatch(ctx))
				b = dup
			}
			nw, werr := dst.Write(b)
			if nw > 0 {
				written += int64(nw)
			}
			if werr != nil {
				return written, werr
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				rerr = nil
			}
			return written, rerr
		}
	}
}

// RoundTrip is a method on the Handler struct that implements the http.RoundTripper interface.
// This method is called by the internal.NewSingleHostReverseProxy to handle the incoming request.
// The method is responsible for validating the incoming request and getting the source ISO.
// If an h.Handler.Patch.SourceISO is a location that redirects to another location, this method handles
// calling the SourceISO location in order to get the final location before returning to the client.
// This is different from the default behavior of a reverse proxy, which is to pass the redirection
// directly back to the client.
func (h *Handler) RoundTrip(req *http.Request) (*http.Response, error) {
	return h.roundTripWithRedirectCount(req, 0)
}

// roundTripWithRedirectCount handles the request with redirect loop protection
func (h *Handler) roundTripWithRedirectCount(req *http.Request, redirectCount int) (*http.Response, error) {
	// Prevent infinite redirect loops
	// The number 10 is arbitrary. 10 was picked as a reasonable limit for most use cases.
	// There doesn't seem to be a standard for this and many client (curl, browsers, etc) seem to have different limits.
	// If a use-case to change this or make it configurable arises, we can revisit.
	const maxRedirects = 10
	if redirectCount > maxRedirects {
		return nil, fmt.Errorf("maximum redirect limit of %d exceeded", maxRedirects)
	}

	log := h.Logger.WithValues("method", req.Method, "inboundURI", req.RequestURI, "remoteAddr", req.RemoteAddr, "redirectCount", redirectCount)
	log.V(1).Info("starting the ISO patching HTTP handler")

	isRedirectRequest := redirectCount > 0

	// Only perform validation on the original incoming request, not on redirected requests
	if !isRedirectRequest {
		if filepath.Ext(req.URL.Path) != ".iso" {
			log.Info("extension not supported, only supported extension is '.iso'", "path", req.URL.Path)
			return &http.Response{
				Status:     fmt.Sprintf("%d %s", http.StatusNotFound, http.StatusText(http.StatusNotFound)),
				StatusCode: http.StatusNotFound,
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}

		// The incoming request url is expected to have the mac address present.
		// Fetch the mac and validate if there's a hardware object
		// associated with the mac.
		//
		// We serve the iso only if this validation passes.
		ha, err := getMAC(req.URL.Path)
		if err != nil {
			log.Info("unable to parse mac address in the URL path", "error", err)
			return &http.Response{
				Status:     fmt.Sprintf("%d %s", http.StatusBadRequest, http.StatusText(http.StatusBadRequest)),
				StatusCode: http.StatusBadRequest,
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}

		fac, hw, err := h.getFacility(req.Context(), ha, h.Backend)
		if err != nil {
			log.Info("unable to get the hardware object", "error", err, "mac", ha.String())
			if apierrors.IsNotFound(err) {
				return &http.Response{
					Status:     fmt.Sprintf("%d %s", http.StatusNotFound, http.StatusText(http.StatusNotFound)),
					StatusCode: http.StatusNotFound,
					Body:       http.NoBody,
					Request:    req,
				}, nil
			}
			return &http.Response{
				Status:     fmt.Sprintf("%d %s", http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)),
				StatusCode: http.StatusInternalServerError,
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}
		// The hardware object doesn't contain a dedicated field for consoles right now and
		// historically the facility is used as a way to define consoles on a per Hardware basis.
		var consoles string
		switch {
		case fac != "" && strings.Contains(fac, "console="):
			consoles = fmt.Sprintf("facility=%s", fac)
		case fac != "":
			consoles = fmt.Sprintf("facility=%s %s", fac, defaultConsoles)
		default:
			consoles = defaultConsoles
		}
		// The patch is added to the request context so that it can be used in the Copy method.
		req = req.WithContext(internal.WithPatch(req.Context(), []byte(h.constructPatch(consoles, ha.String(), hw.DHCP))))

		// Get the target URL (either from query parameter or default SourceISO)
		fromHWObject := ""
		if hw.Isoboot != nil && hw.Isoboot.SourceISO != nil {
			fromHWObject = hw.Isoboot.SourceISO.String()
		}
		tu, err := targetURL(req.URL.Query().Get(queryParamSourceISO), fromHWObject, h.Patch.SourceISO)
		if err != nil {
			log.Info("unable to determine target URL", "error", err)
			return &http.Response{
				Status:     fmt.Sprintf("%d %s", http.StatusBadRequest, http.StatusText(http.StatusBadRequest)),
				StatusCode: http.StatusBadRequest,
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}

		// The internal.NewSingleHostReverseProxy takes the incoming request url and adds the path to the target.
		// This function is more than a pass through proxy. The MAC address in the url path is required to do hardware lookups using the backend reader
		// and is not used when making http calls to the target. All valid requests are passed through to the target.
		req.URL.Path = tu.Path
		req.URL.Host = tu.Host
		req.URL.Scheme = tu.Scheme
	}
	scheme := schemeHTTP
	if req.TLS != nil {
		scheme = schemeHTTPS
	}
	log = log.WithValues("outboundURL", req.URL.String(), "scheme", scheme)

	// RoundTripper needs a Transport to execute a HTTP transaction
	// For our use case the default transport will suffice.
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		log.Error(err, "issue proxying to the source ISO", "url", req.URL.String())
		return nil, err
	}

	// Handle redirect manually
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if location != "" {
			if err := resp.Body.Close(); err != nil {
				log.Error(err, "issue closing redirect response body")
			}

			// Parse and resolve the redirect URL
			redirectURL, err := url.Parse(location)
			if err != nil {
				return nil, fmt.Errorf("failed to parse redirect location: %w", err)
			}

			if !redirectURL.IsAbs() {
				redirectURL = req.URL.ResolveReference(redirectURL)
			}

			// Create new request for redirect
			// This cloned request will have the patch added above
			// so it will be able to patch the request correctly
			redirectReq := req.Clone(req.Context())
			redirectReq.URL = redirectURL

			// Recursively follow the redirect
			return h.roundTripWithRedirectCount(redirectReq, redirectCount+1)
		}
	}

	tu := req.URL.String()
	if resp.StatusCode == http.StatusPartialContent {
		// 0.002% of the time we log a 206 request message.
		// In testing, it was observed that about 3000 HTTP 206 requests are made per ISO mount.
		// 0.002% gives us about 5 - 10, log messages per ISO mount.
		// We're optimizing for showing "enough" log messages so that progress can be observed.
		if p := randomPercentage(100000); p < 0.002 {
			log.Info("206 status code response", "targetURL", tu, "status", resp.Status)
		}
	} else {
		log.Info("response received", "targetURL", tu, "status", resp.Status)
	}

	log.V(1).Info("roundtrip complete")

	return resp, nil
}

func (h *Handler) constructPatch(console, mac string, d *data.DHCP) string {
	syslogHost := fmt.Sprintf("syslog_host=%s", h.Patch.KernelParams.Syslog)
	grpcAuthority := fmt.Sprintf("grpc_authority=%s", h.Patch.KernelParams.TinkServerGRPCAddr)
	tinkerbellTLS := fmt.Sprintf("tinkerbell_tls=%v", h.Patch.KernelParams.TinkServerTLS)
	workerID := fmt.Sprintf("worker_id=%s", mac)
	hwAddr := fmt.Sprintf("hw_addr=%s", mac)
	all := []string{console}
	if d != nil && d.VLANID != "" {
		all = append(all, fmt.Sprintf("vlan_id=%s", d.VLANID))
	}
	all = append(all, hwAddr, syslogHost, grpcAuthority, tinkerbellTLS, workerID)
	all = append(all, h.Patch.KernelParams.ExtraParams...)
	if h.Patch.StaticIPAMEnabled && parseIPAM(d) != "" {
		all = append(all, parseIPAM(d))
	}

	return strings.Join(all, " ")
}

func getMAC(urlPath string) (net.HardwareAddr, error) {
	mac := path.Base(path.Dir(urlPath))
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL path: %s , the second to last element in the URL path must be a valid mac address, err: %w", urlPath, err)
	}

	return hw, nil
}

func (h *Handler) getFacility(ctx context.Context, mac net.HardwareAddr, br BackendReader) (string, data.Hardware, error) {
	if br == nil {
		return "", data.Hardware{}, errors.New("backend is nil")
	}

	hw, err := br.GetByMac(ctx, mac)
	if err != nil {
		return "", data.Hardware{}, err
	}

	return hw.Netboot.Facility, data.Hardware{DHCP: hw.DHCP, Isoboot: hw.Isoboot}, nil
}

func randomPercentage(precision int64) float64 {
	random, err := rand.Int(rand.Reader, big.NewInt(precision))
	if err != nil {
		return 0
	}

	return float64(random.Int64()) / float64(precision)
}
