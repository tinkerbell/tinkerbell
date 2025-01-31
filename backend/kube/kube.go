// Package kube is a backend implementation that uses the Tinkerbell CRDs to get DHCP data.
package kube

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"

	"github.com/ccoveille/go-safecast"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1"
	"github.com/tinkerbell/tinkerbell/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/scale/scheme"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

const tracerName = "github.com/tinkerbell/tinkerbell"

var errNotFound = errors.New("no hardware found")

// ErrInstanceNotFound indicates an instance could not be found for the given identifier.
var ErrInstanceNotFound = errors.New("instance not found")

// Backend is a backend implementation that uses the Tinkerbell CRDs to get DHCP data.
type Backend struct {
	cluster cluster.Cluster
	// ConfigFilePath is the path to a kubernetes config file (kubeconfig).
	ConfigFilePath string
	// APIURL is the Kubernetes API URL.
	APIURL string
	// Namespace is an override for the Namespace the kubernetes client will watch.
	// The default is the Namespace the pod is running in.
	Namespace string
	// ClientConfig is a Kubernetes client config. If specified, it will be used instead of
	// constructing a client using the other configuration in this object. Optional.
	ClientConfig *rest.Config
}

// NewBackend returns a controller-runtime cluster.Cluster with the Tinkerbell runtime
// scheme registered, and indexers for:
// * Hardware by MAC address
// * Hardware by IP address
//
// Callers must instantiate the client-side cache by calling Start() before use.
func NewBackend(cfg Backend, opts ...cluster.Option) (*Backend, error) {
	if cfg.ClientConfig == nil {
		b, err := loadConfig(cfg)
		if err != nil {
			return nil, err
		}
		cfg = b
	}
	rs := runtime.NewScheme()

	if err := scheme.AddToScheme(rs); err != nil {
		return nil, err
	}

	if err := v1alpha1.AddToScheme(rs); err != nil {
		return nil, err
	}
	conf := func(o *cluster.Options) {
		o.Scheme = rs
		if cfg.Namespace != "" {
			o.Cache.DefaultNamespaces = map[string]cache.Config{cfg.Namespace: {}}
		}
	}
	opts = append(opts, conf)
	// remove nils from opts
	sanitizedOpts := make([]cluster.Option, 0, len(opts))
	for _, opt := range opts {
		if opt != nil {
			sanitizedOpts = append(sanitizedOpts, opt)
		}
	}
	c, err := cluster.New(cfg.ClientConfig, sanitizedOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create new cluster config: %w", err)
	}

	if err := c.GetFieldIndexer().IndexField(context.Background(), &v1alpha1.Hardware{}, MACAddrIndex, MACAddrs); err != nil {
		return nil, fmt.Errorf("failed to setup indexer: %w", err)
	}

	if err := c.GetFieldIndexer().IndexField(context.Background(), &v1alpha1.Hardware{}, IPAddrIndex, IPAddrs); err != nil {
		return nil, fmt.Errorf("failed to setup indexer(.spec.interfaces.dhcp.ip.address): %w", err)
	}

	return &Backend{
		cluster:        c,
		ConfigFilePath: cfg.ConfigFilePath,
		APIURL:         cfg.APIURL,
		Namespace:      cfg.Namespace,
		ClientConfig:   cfg.ClientConfig,
	}, nil
}

func loadConfig(cfg Backend) (Backend, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = cfg.ConfigFilePath

	overrides := &clientcmd.ConfigOverrides{
		ClusterInfo: clientcmdapi.Cluster{
			Server: cfg.APIURL,
		},
		Context: clientcmdapi.Context{
			Namespace: cfg.Namespace,
		},
	}

	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	config, err := loader.ClientConfig()
	if err != nil {
		return Backend{}, err
	}
	cfg.ClientConfig = config

	return cfg, nil
}

// Start starts the client-side cache.
func (b *Backend) Start(ctx context.Context) error {
	return b.cluster.Start(ctx)
}

func NewFileRestConfig(kubeconfigPath, namespace string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfigPath

	overrides := &clientcmd.ConfigOverrides{
		ClusterInfo: clientcmdapi.Cluster{
			Server: "",
		},
		Context: clientcmdapi.Context{
			Namespace: namespace,
		},
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	return loader.ClientConfig()
}

// GetByMac implements the handler.BackendReader interface and returns DHCP and netboot data based on a mac address.
func (b *Backend) GetByMac(ctx context.Context, mac net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.GetByMac")
	defer span.End()
	hardwareList := &v1alpha1.HardwareList{}

	if err := b.cluster.GetClient().List(ctx, hardwareList, &client.MatchingFields{MACAddrIndex: mac.String()}); err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed listing hardware for (%v): %w", mac, err)
	}

	if len(hardwareList.Items) == 0 {
		err := hardwareNotFoundError{name: mac.String(), namespace: If(b.Namespace == "", "all namespaces", b.Namespace)}
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}

	if len(hardwareList.Items) > 1 {
		err := fmt.Errorf("got %d hardware objects for mac %s, expected only 1", len(hardwareList.Items), mac)
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}

	i := v1alpha1.Interface{}
	for _, iface := range hardwareList.Items[0].Spec.Interfaces {
		if iface.DHCP.MAC == mac.String() {
			i = iface
			break
		}
	}

	d, n, err := transform(i, hardwareList.Items[0].Spec.Metadata)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}

	span.SetAttributes(d.EncodeToAttributes()...)
	span.SetAttributes(n.EncodeToAttributes()...)
	span.SetStatus(codes.Ok, "")

	return d, n, nil
}

func If[T any](condition bool, valueIfTrue, valueIfFalse T) T {
	if condition {
		return valueIfTrue
	}
	return valueIfFalse
}

// GetByIP implements the handler.BackendReader interface and returns DHCP and netboot data based on an IP address.
func (b *Backend) GetByIP(ctx context.Context, ip net.IP) (*data.DHCP, *data.Netboot, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.GetByIP")
	defer span.End()
	hardwareList := &v1alpha1.HardwareList{}

	if err := b.cluster.GetClient().List(ctx, hardwareList, &client.MatchingFields{IPAddrIndex: ip.String()}); err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed listing hardware for (%v): %w", ip, err)
	}

	if len(hardwareList.Items) == 0 {
		err := hardwareNotFoundError{name: ip.String(), namespace: If(b.Namespace == "", "all namespaces", b.Namespace)}
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}

	if len(hardwareList.Items) > 1 {
		err := fmt.Errorf("got %d hardware objects for ip: %s, expected only 1", len(hardwareList.Items), ip)
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}

	i := v1alpha1.Interface{}
	for _, iface := range hardwareList.Items[0].Spec.Interfaces {
		if iface.DHCP.IP.Address == ip.String() {
			i = iface
			break
		}
	}

	d, n, err := transform(i, hardwareList.Items[0].Spec.Metadata)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}

	span.SetAttributes(d.EncodeToAttributes()...)
	span.SetAttributes(n.EncodeToAttributes()...)
	span.SetStatus(codes.Ok, "")

	return d, n, nil
}

func (b *Backend) hwByIP(ctx context.Context, ip string) (*v1alpha1.Hardware, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.GetByIP")
	defer span.End()
	hardwareList := &v1alpha1.HardwareList{}

	if err := b.cluster.GetClient().List(ctx, hardwareList, &client.MatchingFields{IPAddrIndex: ip}); err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, fmt.Errorf("failed listing hardware for (%v): %w", ip, err)
	}

	if len(hardwareList.Items) == 0 {
		err := hardwareNotFoundError{name: ip, namespace: If(b.Namespace == "", "all namespaces", b.Namespace)}
		span.SetStatus(codes.Error, err.Error())

		return nil, err
	}

	if len(hardwareList.Items) > 1 {
		err := fmt.Errorf("got %d hardware objects for ip: %s, expected only 1", len(hardwareList.Items), ip)
		span.SetStatus(codes.Error, err.Error())

		return nil, err
	}

	span.SetStatus(codes.Ok, "")

	return &hardwareList.Items[0], nil
}

// GetEC2InstanceByIP satisfies ec2.Client.
func (b *Backend) GetEC2Instance(ctx context.Context, ip string) (data.Ec2Instance, error) {
	hw, err := b.hwByIP(ctx, ip)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return data.Ec2Instance{}, ErrInstanceNotFound
		}

		return data.Ec2Instance{}, err
	}

	return toEC2Instance(*hw), nil
}

func toEC2Instance(hw v1alpha1.Hardware) data.Ec2Instance {
	var i data.Ec2Instance

	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil {
		i.Metadata.InstanceID = hw.Spec.Metadata.Instance.ID
		i.Metadata.Hostname = hw.Spec.Metadata.Instance.Hostname
		i.Metadata.LocalHostname = hw.Spec.Metadata.Instance.Hostname
		i.Metadata.Tags = hw.Spec.Metadata.Instance.Tags

		if hw.Spec.Metadata.Instance.OperatingSystem != nil {
			i.Metadata.OperatingSystem.Slug = hw.Spec.Metadata.Instance.OperatingSystem.Slug
			i.Metadata.OperatingSystem.Distro = hw.Spec.Metadata.Instance.OperatingSystem.Distro
			i.Metadata.OperatingSystem.Version = hw.Spec.Metadata.Instance.OperatingSystem.Version
			i.Metadata.OperatingSystem.ImageTag = hw.Spec.Metadata.Instance.OperatingSystem.ImageTag
		}

		// Iterate over all IPs and set the first one for IPv4 and IPv6 as the values in the
		// instance metadata.
		for _, ip := range hw.Spec.Metadata.Instance.Ips {
			// Public IPv4
			if ip.Family == 4 && ip.Public && i.Metadata.PublicIPv4 == "" {
				i.Metadata.PublicIPv4 = ip.Address
			}

			// Private IPv4
			if ip.Family == 4 && !ip.Public && i.Metadata.LocalIPv4 == "" {
				i.Metadata.LocalIPv4 = ip.Address
			}

			// Public IPv6
			if ip.Family == 6 && i.Metadata.PublicIPv6 == "" {
				i.Metadata.PublicIPv6 = ip.Address
			}
		}
	}

	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Facility != nil {
		i.Metadata.Plan = hw.Spec.Metadata.Facility.PlanSlug
		i.Metadata.Facility = hw.Spec.Metadata.Facility.FacilityCode
	}

	if hw.Spec.UserData != nil {
		i.Userdata = *hw.Spec.UserData
	}

	// TODO(chrisdoherty4) Support public keys. The frontend doesn't handle public keys correctly
	// as it expects a single string and just outputs that key. Until we can support multiple keys
	// its not worth adding it to the metadata.
	//
	// https://github.com/tinkerbell/tinkerbell/hegel/issues/165

	return i
}

// toDHCPData converts a v1alpha1.DHCP to a data.DHCP data structure.
// if required fields are missing, an error is returned.
// Required fields: v1alpha1.Interface.DHCP.MAC, v1alpha1.Interface.DHCP.IP.Address, v1alpha1.Interface.DHCP.IP.Netmask.
func toDHCPData(h *v1alpha1.DHCP) (*data.DHCP, error) {
	if h == nil {
		return nil, errors.New("no DHCP data")
	}
	d := new(data.DHCP)

	var err error
	// MACAddress is required
	if d.MACAddress, err = net.ParseMAC(h.MAC); err != nil {
		return nil, err
	}

	if h.IP != nil {
		// IPAddress is required
		if d.IPAddress, err = netip.ParseAddr(h.IP.Address); err != nil {
			return nil, err
		}
		// Netmask is required
		sm := net.ParseIP(h.IP.Netmask)
		if sm == nil {
			return nil, errors.New("no netmask")
		}
		d.SubnetMask = net.IPMask(sm.To4())
	} else {
		return nil, errors.New("no IP data")
	}

	// Gateway is optional, but should be a valid IP address if present
	if h.IP.Gateway != "" {
		if d.DefaultGateway, err = netip.ParseAddr(h.IP.Gateway); err != nil {
			return nil, err
		}
	}

	// name servers, optional
	for _, s := range h.NameServers {
		ip := net.ParseIP(s)
		if ip == nil {
			break
		}
		d.NameServers = append(d.NameServers, ip)
	}

	// timeservers, optional
	for _, s := range h.TimeServers {
		ip := net.ParseIP(s)
		if ip == nil {
			break
		}
		d.NTPServers = append(d.NTPServers, ip)
	}

	// hostname, optional
	d.Hostname = h.Hostname

	// lease time required
	// Default to one week
	d.LeaseTime = 604800
	if v, err := safecast.ToUint32(h.LeaseTime); err == nil {
		d.LeaseTime = v
	}

	// arch
	d.Arch = h.Arch

	// vlanid
	d.VLANID = h.VLANID

	return d, nil
}

// toNetbootData converts a hardware interface to a data.Netboot data structure.
func toNetbootData(i *v1alpha1.Netboot, facility string) (*data.Netboot, error) {
	if i == nil {
		return nil, errors.New("no netboot data")
	}
	n := new(data.Netboot)

	// allow machine to netboot
	if i.AllowPXE != nil {
		n.AllowNetboot = *i.AllowPXE
	}

	// ipxe script url is optional but if provided, it must be a valid url
	if i.IPXE != nil {
		if i.IPXE.URL != "" {
			u, err := url.ParseRequestURI(i.IPXE.URL)
			if err != nil {
				return nil, err
			}
			n.IPXEScriptURL = u
		}
	}

	// ipxescript
	if i.IPXE != nil {
		n.IPXEScript = i.IPXE.Contents
	}

	// console
	n.Console = ""

	// facility
	n.Facility = facility

	// OSIE data
	n.OSIE = data.OSIE{}
	if i.OSIE != nil {
		if b, err := url.Parse(i.OSIE.BaseURL); err == nil {
			n.OSIE.BaseURL = b
		}
		n.OSIE.Kernel = i.OSIE.Kernel
		n.OSIE.Initrd = i.OSIE.Initrd
	}

	return n, nil
}

// transform returns data.DHCP and data.Netboot from part a v1alpha1.Interface and *v1alpha1.HardwareMetadata.
func transform(i v1alpha1.Interface, m *v1alpha1.HardwareMetadata) (*data.DHCP, *data.Netboot, error) {
	d, err := toDHCPData(i.DHCP)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert hardware to DHCP data: %w", err)
	}
	d.Disabled = i.DisableDHCP

	// Facility is used in the default HookOS iPXE script so we get it from the hardware metadata, if set.
	facility := ""
	if m != nil {
		if m.Facility != nil {
			facility = m.Facility.FacilityCode
		}
	}

	n, err := toNetbootData(i.Netboot, facility)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert hardware to netboot data: %w", err)
	}

	return d, n, nil
}
