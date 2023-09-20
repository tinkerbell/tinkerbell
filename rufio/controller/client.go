package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"dario.cat/mergo"
	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/bmclib/v2/providers/rpc"
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
		o := opts.translate(hostIP)
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

func (b BMCOptions) translate(host string) []bmclib.Option {
	o := []bmclib.Option{}

	if b.ProviderOptions == nil {
		return o
	}

	// redfish options
	if b.Redfish != nil {
		if b.Redfish.Port != 0 {
			o = append(o, bmclib.WithRedfishPort(strconv.Itoa(b.Redfish.Port)))
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
		amt := bmclib.WithIntelAMTPort(uint32(b.IntelAMT.Port))
		o = append(o, amt)
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
		Opts: rpc.Opts{
			Request: rpc.RequestOpts{
				HTTPContentType: b.RPC.Request.HTTPContentType,
				HTTPMethod:      b.RPC.Request.HTTPMethod,
				StaticHeaders:   b.RPC.Request.StaticHeaders,
				TimestampFormat: b.RPC.Request.TimestampFormat,
				TimestampHeader: b.RPC.Request.TimestampHeader,
			},
			Signature: rpc.SignatureOpts{
				HeaderName:                 b.RPC.Signature.HeaderName,
				AppendAlgoToHeaderDisabled: b.RPC.Signature.AppendAlgoToHeaderDisabled,
				IncludedPayloadHeaders:     b.RPC.Signature.IncludedPayloadHeaders,
			},
			HMAC: rpc.HMACOpts{
				PrefixSigDisabled: b.RPC.HMAC.PrefixSigDisabled,
				Secrets:           s,
			},
			Experimental: rpc.Experimental{
				CustomRequestPayload: []byte(b.RPC.Experimental.CustomRequestPayload),
				DotPath:              b.RPC.Experimental.DotPath,
			},
		},
	}

	_ = mergo.Merge(&o, &defaults, mergo.WithOverride, mergo.WithTransformers(&rpc.Provider{}))

	return o
}
