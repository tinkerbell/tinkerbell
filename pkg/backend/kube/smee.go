package kube

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"

	"github.com/ccoveille/go-safecast"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
		err := hardwareNotFoundError{name: mac.String(), namespace: ternary(b.Namespace == "", "all namespaces", b.Namespace)}
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

func ternary[T any](condition bool, valueIfTrue, valueIfFalse T) T {
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
		err := hardwareNotFoundError{name: ip.String(), namespace: ternary(b.Namespace == "", "all namespaces", b.Namespace)}
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

// toDHCPData converts a v1alpha1.DHCP to a data.DHCP data structure.
// Fields that are set are checked for correctness of their types.
func toDHCPData(h *v1alpha1.DHCP) (*data.DHCP, error) {
	if h == nil {
		return nil, errors.New("no DHCP data")
	}
	d := new(data.DHCP)

	var err error
	if h.MAC != "" {
		// A valid MACAddress is required
		if d.MACAddress, err = net.ParseMAC(h.MAC); err != nil {
			return nil, err
		}
	}

	if h.IP != nil {
		// A valid IPAddress is required
		if d.IPAddress, err = netip.ParseAddr(h.IP.Address); err != nil {
			return nil, err
		}
		// A valid Netmask is required
		sm := net.ParseIP(h.IP.Netmask)
		if sm == nil {
			return nil, errors.New("no netmask")
		}
		d.SubnetMask = net.IPMask(sm.To4())
	}

	// Gateway is optional, but should be a valid IP address if present
	if h.IP != nil && h.IP.Gateway != "" {
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
