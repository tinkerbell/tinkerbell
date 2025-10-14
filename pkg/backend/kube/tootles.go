package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetHackInstance returns a hack.Instance by calling the hwByIP method and converting the result.
// This is a method that the Tootles service uses.
func (b *Backend) GetHackInstance(ctx context.Context, ip string) (data.HackInstance, error) {
	hw, err := b.hwByIP(ctx, ip)
	if err != nil {
		return data.HackInstance{}, err
	}

	return toHackInstance(*hw)
}

// toHackInstance converts a Tinkerbell Hardware resource to a hack.Instance by marshalling and
// unmarshalling. This works because the Hardware resource has historical roots that align with
// the hack.Instance struct that is derived from the rootio action. See the hack frontend for more
// details.
func toHackInstance(hw v1alpha1.Hardware) (data.HackInstance, error) {
	marshalled, err := json.Marshal(hw.Spec)
	if err != nil {
		return data.HackInstance{}, err
	}

	var i data.HackInstance
	if err := json.Unmarshal(marshalled, &i); err != nil {
		return data.HackInstance{}, err
	}

	return i, nil
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

// GetNoCloudInstance returns a NoCloudInstance by calling the hwByIP method and converting the result.
// This is a method that the Tootles service uses.
func (b *Backend) GetNoCloudInstance(ctx context.Context, ip string) (data.NoCloudInstance, error) {
	hw, err := b.hwByIP(ctx, ip)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return data.NoCloudInstance{}, ErrInstanceNotFound
		}

		return data.NoCloudInstance{}, err
	}

	return b.toNoCloudInstance(*hw), nil
}

// toNoCloudInstance converts a Tinkerbell Hardware resource to a NoCloudInstance.
func (b *Backend) toNoCloudInstance(hw v1alpha1.Hardware) data.NoCloudInstance {
	var i data.NoCloudInstance

	// Set metadata from Hardware resource
	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil {
		i.Metadata.InstanceID = hw.Spec.Metadata.Instance.ID
		i.Metadata.LocalHostname = hw.Spec.Metadata.Instance.Hostname
	}

	// Set user data from Hardware resource
	if hw.Spec.UserData != nil {
		i.Userdata = *hw.Spec.UserData
	}

	// Generate network configuration from Hardware resource
	i.NetworkConfig = generateNetworkConfigV2(hw)

	return i
}

// getNameServers extracts nameservers from Hardware interfaces.
// Returns IPv4 and IPv6 nameservers separately.
func getNameServers(hw v1alpha1.Hardware) (ipv4DNS []string, ipv6DNS []string) {
	// Try to get nameservers from the first interface with DHCP config
	for _, iface := range hw.Spec.Interfaces {
		if iface.DHCP != nil && len(iface.DHCP.NameServers) > 0 {
			// Separate IPv4 and IPv6 nameservers
			for _, ns := range iface.DHCP.NameServers {
				if strings.Contains(ns, ":") {
					ipv6DNS = append(ipv6DNS, ns)
				} else {
					ipv4DNS = append(ipv4DNS, ns)
				}
			}
			break
		}
	}

	return ipv4DNS, ipv6DNS
}

func cidrFromNetmask(netmask string) string {
	if netmask == "" {
		return ""
	}

	parts := strings.Split(netmask, ".")
	if len(parts) != 4 {
		return ""
	}

	setBits := 0
	for _, part := range parts {
		octet := 0
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return ""
			}
			octet = octet*10 + int(ch-'0')
		}

		if octet < 0 || octet > 255 {
			return ""
		}

		for octet > 0 {
			setBits++
			octet &= (octet - 1)
		}
	}

	if setBits < 0 || setBits > 32 {
		return ""
	}

	return fmt.Sprintf("%d", setBits)
}

// generateNetworkConfigV2 creates a NoCloud-compatible network configuration (version 2) from Hardware resource.
// Version 2 is the modern Netplan-compatible format.
// Only generates configuration for network bonding. For non-bonded interfaces, cloud-init handles default DHCP.
func generateNetworkConfigV2(hw v1alpha1.Hardware) interface{} {
	config := map[string]interface{}{
		"network": map[string]interface{}{
			"version": 2,
		},
	}

	network, ok := config["network"].(map[string]interface{})
	if !ok {
		// This should never happen since we just created it, but satisfy the linter
		return config
	}

	// Check if bonding is enabled
	bondingEnabled := hw.Spec.Metadata != nil && hw.Spec.Metadata.BondingMode > 0

	if bondingEnabled && len(hw.Spec.Interfaces) >= 2 {
		// Generate bonding configuration
		ethernets, bonds := generateBondingConfigurationV2(hw)
		network["ethernets"] = ethernets
		network["bonds"] = bonds
	}

	return config
}

// generateBondingConfigurationV2 creates bonding configuration (v2 format) from Hardware resource.
// Returns separate ethernets and bonds maps.
// Requires MAC addresses for all interfaces to enable proper matching.
func generateBondingConfigurationV2(hw v1alpha1.Hardware) (map[string]interface{}, map[string]interface{}) {
	ethernets := map[string]interface{}{}
	bonds := map[string]interface{}{}
	bondInterfaces := []string{}

	// Create physical interfaces for bonding (without IP addresses)
	phyIndex := 0
	for _, iface := range hw.Spec.Interfaces {
		if iface.DHCP == nil || iface.DHCP.MAC == "" {
			continue
		}

		// Use bond0phyX naming for interface references
		interfaceName := fmt.Sprintf("bond0phy%d", phyIndex)
		phyIndex++
		bondInterfaces = append(bondInterfaces, interfaceName)

		ethernetConfig := map[string]interface{}{
			"dhcp4": false,
			"match": map[string]interface{}{
				"macaddress": iface.DHCP.MAC,
			},
			"set-name": interfaceName,
		}

		ethernets[interfaceName] = ethernetConfig
	}

	// Create bond configuration
	bondConfig := map[string]interface{}{
		"interfaces": bondInterfaces,
		"parameters": generateBondParametersV2(hw.Spec.Metadata.BondingMode),
	}

	// Add IP configuration to bond
	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil && len(hw.Spec.Metadata.Instance.Ips) > 0 {
		ipv4DNS, ipv6DNS := getNameServers(hw)
		addresses, gateway4, gateway6, nameservers := generateAddressConfigV2(hw.Spec.Metadata.Instance.Ips, ipv4DNS, ipv6DNS)

		bondConfig["addresses"] = addresses
		if gateway4 != "" {
			bondConfig["gateway4"] = gateway4
		}
		if gateway6 != "" {
			bondConfig["gateway6"] = gateway6
		}
		if len(nameservers) > 0 {
			bondConfig["nameservers"] = map[string]interface{}{
				"addresses": nameservers,
			}
		}
	} else {
		// Default to DHCP if no static IPs (IPv4 only)
		bondConfig["dhcp4"] = true
	}

	bonds["bond0"] = bondConfig
	return ethernets, bonds
}

// generateBondParametersV2 creates bonding parameters (v2 format) based on bonding mode.
// V2 format uses hyphenated names without the "bond-" prefix.
func generateBondParametersV2(bondingMode int64) map[string]interface{} {
	params := map[string]interface{}{
		"mii-monitor-interval": 100,
	}

	switch bondingMode {
	case 0:
		params["mode"] = "balance-rr"
	case 1:
		params["mode"] = "active-backup"
		params["primary-reselect-policy"] = "always"
		params["fail-over-mac-policy"] = "none"
	case 2:
		params["mode"] = "balance-xor"
		params["transmit-hash-policy"] = "layer2"
	case 3:
		params["mode"] = "broadcast"
	case 4:
		params["mode"] = "802.3ad"
		params["lacp-rate"] = "fast"
		params["transmit-hash-policy"] = "layer3+4"
		params["ad-select"] = "stable"
	case 5:
		params["mode"] = "balance-tlb"
	case 6:
		params["mode"] = "balance-alb"
	default:
		// Default to active-backup for unknown modes
		params["mode"] = "active-backup"
		params["primary-reselect-policy"] = "always"
	}

	return params
}

// generateAddressConfigV2 creates address configuration (v2 format) from IP metadata.
// Returns addresses array, gateway4, gateway6, and combined nameservers list.
func generateAddressConfigV2(ips []*v1alpha1.MetadataInstanceIP, ipv4DNS []string, ipv6DNS []string) ([]string, string, string, []string) {
	addresses := []string{}
	gateway4 := ""
	gateway6 := ""
	nameservers := []string{}

	// Combine nameservers (IPv4 first, then IPv6)
	nameservers = append(nameservers, ipv4DNS...)
	nameservers = append(nameservers, ipv6DNS...)

	for _, ip := range ips {
		switch ip.Family {
		case 4:
			addresses = append(addresses, fmt.Sprintf("%s/%s", ip.Address, cidrFromNetmask(ip.Netmask)))
			// Set gateway4 from the first IPv4 with a gateway
			if gateway4 == "" && ip.Gateway != "" {
				gateway4 = ip.Gateway
			}
		case 6:
			addresses = append(addresses, ip.Address)
			// Set gateway6 from the first IPv6 with a gateway
			if gateway6 == "" && ip.Gateway != "" {
				gateway6 = ip.Gateway
			}
		}
	}

	// If no addresses found, return empty (will use DHCP)
	if len(addresses) == 0 {
		return addresses, "", "", []string{}
	}

	return addresses, gateway4, gateway6, nameservers
}

func (b *Backend) hwByIP(ctx context.Context, ip string) (*v1alpha1.Hardware, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.hwByIP")
	defer span.End()
	hardwareList := &v1alpha1.HardwareList{}

	if err := b.cluster.GetClient().List(ctx, hardwareList, &client.MatchingFields{IPAddrIndex: ip}); err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, fmt.Errorf("failed listing hardware for (%v): %w", ip, err)
	}

	if len(hardwareList.Items) == 0 {
		err := hardwareNotFoundError{name: ip, namespace: ternary(b.Namespace == "", "all namespaces", b.Namespace)}
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
