package bmc

import (
	"net/http"
)

const (
	ProviderNameIPMITool   ProviderName = "ipmitool"
	ProviderNameAsrockRack ProviderName = "asrockrack"
	ProviderNameGofish     ProviderName = "gofish"
	ProviderNameIntelAMT   ProviderName = "IntelAMT"
	ProviderNameDell       ProviderName = "dell"
	ProviderNameSupermicro ProviderName = "supermicro"
	ProviderNameOpenBMC    ProviderName = "openbmc"
)

// ProviderOptions holds provider specific configurable options.
type ProviderOptions struct {
	// PreferredOrder allows customizing the order that BMC providers are called.
	// Providers added to this list will be moved to the front of the default order.
	// Provider names are case insensitive.
	// The default order is: ipmitool, asrockrack, gofish, intelamt, dell, supermicro, openbmc.
	// +optional
	// +listType=atomic
	PreferredOrder []ProviderName `json:"preferredOrder,omitempty"`
	// IntelAMT contains the options to customize the IntelAMT provider.
	// +optional
	IntelAMT *IntelAMTOptions `json:"intelAMT,omitempty"`

	// IPMITOOL contains the options to customize the Ipmitool provider.
	// +optional
	IPMITOOL *IPMITOOLOptions `json:"ipmitool,omitempty"`

	// Redfish contains the options to customize the Redfish provider.
	// +optional
	Redfish *RedfishOptions `json:"redfish,omitempty"`

	// RPC contains the options to customize the RPC provider.
	// +optional
	RPC *RPCOptions `json:"rpc,omitempty"`

	// HomeAssistant contains the options to customize the HomeAssistant provider.
	// +optional
	HomeAssistant *HomeAssistantOptions `json:"homeAssistant,omitempty"`
}

// ProviderName is the bmclib specific provider name. Names are case insensitive.
// +kubebuilder:validation:Pattern=(?i)^(ipmitool|asrockrack|gofish|IntelAMT|dell|supermicro|openbmc)$
type ProviderName string

func (p ProviderName) String() string {
	return string(p)
}

// HomeAssistantOptions has the config options for the Home Assistant provider. Make me better!
type HomeAssistantOptions struct {
	// SwitchEntityID is the entity ID of the Home Assistant switch entity to control power state
	// +required
	SwitchEntityID string `json:"switchEntityID"`
	// PowerOperationDelaySeconds is the number of seconds to wait after a power operation before proceeding
	// +kubebuilder:validation:Minimum=0
	// +optional
	PowerOperationDelaySeconds int32 `json:"powerOperationDelaySeconds,omitempty"`
}

// RedfishOptions contains the redfish provider specific options.
type RedfishOptions struct {
	// Port that redfish will use for calls.
	// +optional
	Port int32 `json:"port,omitempty"`
	// UseBasicAuth for redfish calls. The default is false which means token based auth is used.
	// Unset is equivalent to false.
	// +optional
	UseBasicAuth *bool `json:"useBasicAuth,omitempty"`
	// SystemName is the name of the system to use for redfish calls.
	// With redfish implementations that manage multiple systems via a single endpoint, this allows for specifying the system to manage.
	// +optional
	SystemName string `json:"systemName,omitempty"`
	// EtagMatchDisabled stops the redfish provider from sending a resource's ETag in an If-Match header
	// on PATCH and POST requests. Some BMC implementations reject a request whose If-Match doesn't
	// match the ETag they expect, with 412 Precondition Failed, even when the ETag came from a read just before.
	// Unset is equivalent to false.
	// +optional
	EtagMatchDisabled *bool `json:"etagMatchDisabled,omitempty"`
}

// IPMITOOLOptions contains the ipmitool provider specific options.
type IPMITOOLOptions struct {
	// Port that ipmitool will use for calls.
	// +optional
	Port int32 `json:"port,omitempty"`
	// CipherSuite that ipmitool will use for calls.
	// +optional
	CipherSuite string `json:"cipherSuite,omitempty"`
}

// IntelAMTOptions contains the intelAMT provider specific options.
type IntelAMTOptions struct {
	// Port that intelAMT will use for calls.
	// +optional
	Port int32 `json:"port,omitempty"`

	// HostScheme determines whether to use http or https for intelAMT calls.
	// +optional
	// +kubebuilder:validation:Enum=http;https
	// +kubebuilder:default:=http
	HostScheme string `json:"hostScheme,omitempty"`
}

// HMACAlgorithm is a type for HMAC algorithms.
type HMACAlgorithm string

// HMACSecrets holds per Algorithm slice secrets.
// These secrets will be used to create HMAC signatures.
type HMACSecrets map[HMACAlgorithm][]SimpleReference

// RPCOptions defines the configurable options to use when sending rpc notifications.
type RPCOptions struct {
	// ConsumerURL is the URL where an rpc consumer/listener is running
	// and to which we will send and receive all notifications.
	// +required
	ConsumerURL string `json:"consumerURL"`
	// LogNotificationsDisabled determines whether responses from rpc consumer/listeners will be logged or not.
	// Unset is equivalent to false.
	// +optional
	LogNotificationsDisabled *bool `json:"logNotificationsDisabled,omitempty"`
	// Request is the options used to create the rpc HTTP request.
	// +optional
	Request *RequestOpts `json:"request,omitempty"`
	// Signature is the options used for adding an HMAC signature to an HTTP request.
	// +optional
	Signature *SignatureOpts `json:"signature,omitempty"`
	// HMAC is the options used to create a HMAC signature.
	// +optional
	HMAC *HMACOpts `json:"hmac,omitempty"`
	// Experimental options.
	// +optional
	Experimental *ExperimentalOpts `json:"experimental,omitempty"`
}

// RequestOpts are the options used when creating an HTTP request.
type RequestOpts struct {
	// HTTPContentType is the content type to use for the rpc request notification.
	// +optional
	HTTPContentType string `json:"httpContentType,omitempty"`
	// HTTPMethod is the HTTP method to use for the rpc request notification.
	// +optional
	HTTPMethod string `json:"httpMethod,omitempty"`
	// StaticHeaders are predefined headers that will be added to every request.
	// +optional
	StaticHeaders http.Header `json:"staticHeaders,omitempty"`
	// TimestampFormat is the time format for the timestamp header.
	// +optional
	TimestampFormat string `json:"timestampFormat,omitempty"`
	// TimestampHeader is the header name that should contain the timestamp. Example: X-BMCLIB-Timestamp
	// +optional
	TimestampHeader string `json:"timestampHeader,omitempty"`
}

// SignatureOpts are the options used for adding an HMAC signature to an HTTP request.
type SignatureOpts struct {
	// HeaderName is the header name that should contain the signature(s). Example: X-BMCLIB-Signature
	// +optional
	HeaderName string `json:"headerName,omitempty"`
	// AppendAlgorithmToHeaderDisabled decides whether to append the algorithm to the signature header or not.
	// Example: X-BMCLIB-Signature becomes X-BMCLIB-Signature-256
	// When set to true, a header will be added for each algorithm. Example: X-BMCLIB-Signature-256 and X-BMCLIB-Signature-512
	// Unset is equivalent to false.
	// +optional
	AppendAlgorithmToHeaderDisabled *bool `json:"appendAlgorithmToHeaderDisabled,omitempty"`
	// IncludedPayloadHeaders are headers whose values will be included in the signature payload. Example: X-BMCLIB-My-Custom-Header
	// All headers will be deduplicated.
	// +optional
	// +listType=atomic
	IncludedPayloadHeaders []string `json:"includedPayloadHeaders,omitempty"`
}

// HMACOpts are the options used to create an HMAC signature.
type HMACOpts struct {
	// AlgorithmPrefixDisabled determines whether the algorithm will be prefixed to the signature. Example: sha256=abc123
	// Unset is equivalent to false.
	// +optional
	AlgorithmPrefixDisabled *bool `json:"algorithmPrefixDisabled,omitempty"`
	// Secrets are a map of algorithms to secrets used for signing.
	// +optional
	Secrets HMACSecrets `json:"secrets,omitempty"`
}

// ExperimentalOpts are options we're still learning about and should be used carefully.
type ExperimentalOpts struct {
	// CustomRequestPayload must be in json.
	// +optional
	CustomRequestPayload string `json:"customRequestPayload,omitempty"`
	// DotPath is the path to the json object where the bmclib RequestPayload{} struct will be embedded. For example: object.data.body
	// +optional
	DotPath string `json:"dotPath,omitempty"`
}
