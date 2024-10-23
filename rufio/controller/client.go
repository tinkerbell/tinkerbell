package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"dario.cat/mergo"
	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/bmclib/v2/providers/rpc"
	"github.com/ccoveille/go-safecast"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/rufio/api/v1alpha1"
)

// ClientFunc defines a func that returns a bmclib.Client.
type ClientFunc func(ctx context.Context, log logr.Logger, hostIP, username, password string, opts *BMCOptions) (*bmclib.Client, error)

// NewClientFunc returns a new BMCClientFactoryFunc. The timeout parameter determines the
// maximum time to probe for compatible interfaces.
func NewClientFunc(timeout time.Duration) ClientFunc {
	// Initializes a bmclib client based on input host and credentials
	// Establishes a connection with the bmc with client.Open
	// Returns a bmclib.Client.
	return func(ctx context.Context, log logr.Logger, hostIP, username, password string, opts *BMCOptions) (*bmclib.Client, error) {
		o := opts.Translate(hostIP)
		log = log.WithValues("host", hostIP, "username", username)
		o = append(o, bmclib.WithLogger(log))
		client := bmclib.NewClient(hostIP, username, password, o...)

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// TODO (pokearu): Make an option
		client.Registry.Drivers = client.Registry.PreferProtocol("redfish")
		if err := client.Open(ctx); err != nil {
			md := client.GetMetadata()
			log.Info("Failed to open connection to BMC", "error", err, "providersAttempted", md.ProvidersAttempted, "successfulProvider", md.SuccessfulOpenConns)

			return nil, fmt.Errorf("failed to open connection to BMC: %w", err)
		}
		md := client.GetMetadata()
		log.Info("Connected to BMC", "providersAttempted", md.ProvidersAttempted, "successfulProvider", md.SuccessfulOpenConns)

		return client, nil
	}
}

type BMCOptions struct {
	*v1alpha1.ProviderOptions
	rpcSecrets map[rpc.Algorithm][]string
}

func (b BMCOptions) Translate(host string) []bmclib.Option {
	o := []bmclib.Option{}

	if b.ProviderOptions == nil {
		return o
	}

	// redfish options
	if b.Redfish != nil {
		if b.Redfish.Port != 0 {
			o = append(o, bmclib.WithRedfishPort(strconv.Itoa(b.Redfish.Port)))
		}
		if b.Redfish.UseBasicAuth {
			o = append(o, bmclib.WithRedfishUseBasicAuth(true))
		}
		if b.Redfish.SystemName != "" {
			o = append(o, bmclib.WithRedfishSystemName(b.Redfish.SystemName))
		}
	}

	// ipmitool options
	if b.IPMITOOL != nil {
		if b.IPMITOOL.Port != 0 {
			o = append(o, bmclib.WithIpmitoolPort(strconv.Itoa(b.IPMITOOL.Port)))
		}
		if b.IPMITOOL.CipherSuite != "" {
			o = append(o, bmclib.WithIpmitoolCipherSuite(b.IPMITOOL.CipherSuite))
		}
	}

	// intelAmt options
	if b.IntelAMT != nil {
		// must not be negative, must not be greater than the uint32 max value
		p, err := safecast.ToUint32(b.IntelAMT.Port)
		if err != nil {
			p = 16992
		}
		amtPort := bmclib.WithIntelAMTPort(p)
		amtScheme := bmclib.WithIntelAMTHostScheme(b.IntelAMT.HostScheme)
		o = append(o, amtPort, amtScheme)
	}

	// rpc options
	if b.RPC != nil {
		op := b.translateRPC(host)
		o = append(o, bmclib.WithRPCOpt(op))
	}

	return o
}

func (b BMCOptions) translateRPC(host string) rpc.Provider {
	s := map[rpc.Algorithm][]string{}
	if b.rpcSecrets != nil {
		s = b.rpcSecrets
	}

	defaults := rpc.Provider{
		Opts: rpc.Opts{
			Request: rpc.RequestOpts{
				TimestampHeader: "X-Rufio-Timestamp",
			},
			Signature: rpc.SignatureOpts{
				HeaderName:             "X-Rufio-Signature",
				IncludedPayloadHeaders: []string{"X-Rufio-Timestamp"},
			},
		},
	}
	o := rpc.Provider{
		ConsumerURL: b.RPC.ConsumerURL,
		Host:        host,
		Opts:        toRPCOpts(b.RPC),
	}
	if len(s) > 0 {
		o.Opts.HMAC.Secrets = s
	}

	_ = mergo.Merge(&o, &defaults, mergo.WithOverride, mergo.WithTransformers(&rpc.Provider{}))

	return o
}

func toRPCOpts(r *v1alpha1.RPCOptions) rpc.Opts {
	opt := rpc.Opts{}

	if r == nil {
		return opt
	}
	opt.Request = toRequestOpts(r.Request)
	opt.Signature = toSignatureOpts(r.Signature)
	opt.HMAC = toHMACOpts(r.HMAC)
	opt.Experimental = toExperimentalOpts(r.Experimental)

	return opt
}

func toRequestOpts(r *v1alpha1.RequestOpts) rpc.RequestOpts {
	opt := rpc.RequestOpts{}
	if r == nil {
		return opt
	}
	if r.HTTPContentType != "" {
		opt.HTTPContentType = r.HTTPContentType
	}
	if r.HTTPMethod != "" {
		opt.HTTPMethod = r.HTTPMethod
	}
	if len(r.StaticHeaders) > 0 {
		opt.StaticHeaders = r.StaticHeaders
	}
	if r.TimestampFormat != "" {
		opt.TimestampFormat = r.TimestampFormat
	}
	if r.TimestampHeader != "" {
		opt.TimestampHeader = r.TimestampHeader
	}

	return opt
}

func toSignatureOpts(s *v1alpha1.SignatureOpts) rpc.SignatureOpts {
	opt := rpc.SignatureOpts{}

	if s == nil {
		return opt
	}
	if s.HeaderName != "" {
		opt.HeaderName = s.HeaderName
	}
	if s.AppendAlgoToHeaderDisabled {
		opt.AppendAlgoToHeaderDisabled = s.AppendAlgoToHeaderDisabled
	}
	if len(s.IncludedPayloadHeaders) > 0 {
		opt.IncludedPayloadHeaders = s.IncludedPayloadHeaders
	}

	return opt
}

func toHMACOpts(h *v1alpha1.HMACOpts) rpc.HMACOpts {
	opt := rpc.HMACOpts{}

	if h == nil {
		return opt
	}
	if h.PrefixSigDisabled {
		opt.PrefixSigDisabled = h.PrefixSigDisabled
	}

	return opt
}

func toExperimentalOpts(e *v1alpha1.ExperimentalOpts) rpc.Experimental {
	opt := rpc.Experimental{}

	if e == nil {
		return opt
	}
	if e.CustomRequestPayload != "" {
		opt.CustomRequestPayload = []byte(e.CustomRequestPayload)
	}
	if e.DotPath != "" {
		opt.DotPath = e.DotPath
	}

	return opt
}
