package v1alpha1

import (
	"net/http"

	corev1 "k8s.io/api/core/v1"
)

// Config defines the configuration for sending rpc notifications.
type OptRPC struct {
	// ConsumerURL is the URL where an rpc consumer/listener is running
	// and to which we will send and receive all notifications.
	ConsumerURL string `json:"consumerURL"`
	// LogNotificationsDisabled determines whether responses from rpc consumer/listeners will be logged or not.
	LogNotificationsDisabled bool `json:"logNotificationsDisabled"`
	// Opts are the options for the rpc provider.
	Opts Opts `json:"opts"`
}

// Opts are the options for the rpc provider.
type Opts struct {
	// Request is the options used to create the rpc HTTP request.
	Request RequestOpts `json:"request"`
	// Signature is the options used for adding an HMAC signature to an HTTP request.
	Signature SignatureOpts `json:"signature"`
	// HMAC is the options used to create a HMAC signature.
	HMAC HMACOpts `json:"hmac"`
	// Experimental options.
	Experimental Experimental `json:"experimental"`
}

// RequestOpts are the options used to create the rpc HTTP request.
type RequestOpts struct {
	// HTTPContentType is the content type to use for the rpc request notification.
	HTTPContentType string `json:"httpContentType"`
	// HTTPMethod is the HTTP method to use for the rpc request notification.
	HTTPMethod string `json:"httpMethod"`
	// StaticHeaders are predefined headers that will be added to every request.
	StaticHeaders http.Header `json:"staticHeaders"`
	// TimestampFormat is the time format for the timestamp header.
	TimestampFormat string `json:"timestampFormat"`
	// TimestampHeader is the header name that should contain the timestamp. Example: X-BMCLIB-Timestamp
	TimestampHeader string `json:"timestampHeader"`
}

// SignatureOpts are the options used for adding an HMAC signature to an HTTP request.
type SignatureOpts struct {
	// HeaderName is the header name that should contain the signature(s). Example: X-BMCLIB-Signature
	HeaderName string `json:"headerName"`
	// AppendAlgoToHeaderDisabled decides whether to append the algorithm to the signature header or not.
	// Example: X-BMCLIB-Signature becomes X-BMCLIB-Signature-256
	// When set to true, a header will be added for each algorithm. Example: X-BMCLIB-Signature-256 and X-BMCLIB-Signature-512
	AppendAlgoToHeaderDisabled bool `json:"appendAlgoToHeaderDisabled"`
	// IncludedPayloadHeaders are headers whose values will be included in the signature payload. Example: X-BMCLIB-My-Custom-Header
	// All headers will be deduplicated.
	IncludedPayloadHeaders []string `json:"includedPayloadHeaders"`
}

// HMACOpts are the options used to create a HMAC signature.
type HMACOpts struct {
	// PrefixSigDisabled determines whether the algorithm will be prefixed to the signature. Example: sha256=abc123
	PrefixSigDisabled bool `json:"prefixSigDisabled"`
	// Secrets are a map of algorithms to secrets used for signing.
	Secrets Secrets `json:"secrets"`
}

// Experimental options.
type Experimental struct {
	// CustomRequestPayload must be in json.
	CustomRequestPayload string `json:"customRequestPayload"`
	// DotPath is the path to where the bmclib RequestPayload{} will be embedded. For example: object.data.body
	DotPath string `json:"dotPath"`
}

// Algorithm is the type for HMAC algorithms.
type Algorithm string

// Secrets hold per algorithm slice secrets.
// These secrets will be used to create HMAC signatures.
type Secrets map[Algorithm][]corev1.SecretReference

// OptRedfish contains redfish provider options.
type OptRedfish struct {
	Port int `json:"port"`
}

// OptIPMITOOL contains ipmitool provider options.
type OptIPMITOOL struct {
	Port        int    `json:"port"`
	CipherSuite string `json:"cipherSuite"`
}

// OptIntelAMT contains intel amt provider options.
type OptIntelAMT struct {
	Port int `json:"port"`
}
