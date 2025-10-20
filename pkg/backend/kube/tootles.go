package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// bondInterfacePattern matches bond interface names like "bond0phy0", "bond1phy2".
// Captures: bond number, phy number.
var bondInterfacePattern = regexp.MustCompile(`^bond(\d+)phy(\d+)$`)

// bondTagPattern matches bond mode tags like "bond0:mode4".
// Captures: bond name (with number), mode (0-6).
var bondTagPattern = regexp.MustCompile(`^(bond\d+):mode([0-6])$`)

// bondConfig holds configuration for a single bond.
// IP and nameservers are taken from the first member (phy0).
type bondConfig struct {
	name        string
	mode        int64
	interfaces  []v1alpha1.Interface
	ip          *v1alpha1.IP
	nameservers []string
}

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
		i.Metadata.Hostname = hw.Spec.Metadata.Instance.Hostname
		i.Metadata.LocalHostname = hw.Spec.Metadata.Instance.Hostname
		i.Metadata.Tags = hw.Spec.Metadata.Instance.Tags
		i.Metadata.PublicKeys = hw.Spec.Metadata.Instance.SSHKeys

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

// inferIPFamily infers the IP family (4 or 6) from an IP address if not explicitly set.
// Returns the original family if non-zero, otherwise infers from address format.
// Returns 0 if the address is invalid or empty.
func inferIPFamily(family int64, address string) int64 {
	if family != 0 || address == "" {
		return family
	}

	if ip := net.ParseIP(address); ip != nil {
		if ip.To4() != nil {
			return 4 // IPv4
		} else if ip.To16() != nil {
			return 6 // IPv6
		}
	}

	return 0 // Invalid or unparseable address
}

// parseBondModeFromTags extracts bond modes from instance tags.
// Expected format: "bond<N>:mode<M>" where N is bond number, M is mode 0-6.
// Example: "bond0:mode4" configures bond0 with mode 4 (802.3ad).
// Returns map of bond name to mode number.
func parseBondModeFromTags(tags []string) map[string]int64 {
	bondModes := make(map[string]int64)

	for _, tag := range tags {
		matches := bondTagPattern.FindStringSubmatch(tag)
		if len(matches) != 3 {
			continue
		}

		bondName := matches[1]
		mode, _ := strconv.ParseInt(matches[2], 10, 64)
		bondModes[bondName] = mode
	}

	return bondModes
}

// parseBondConfigurations groups interfaces into bonds based on IfaceName pattern "bond<N>phy<M>".
// Bond mode priority: tags (bond<N>:mode<M>) > Metadata.BondingMode > mode 1 (active-backup).
// IP configuration and nameservers are taken from the first member (phy0) of each bond.
// Returns map of bond name to bondConfig.
func parseBondConfigurations(hw v1alpha1.Hardware) map[string]*bondConfig {
	bonds := make(map[string]*bondConfig)

	// Parse bond modes from instance tags
	var bondModes map[string]int64
	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil {
		bondModes = parseBondModeFromTags(hw.Spec.Metadata.Instance.Tags)
	}

	// Get default bonding mode from metadata, or use mode 1 (active-backup) if not set
	defaultMode := int64(1)
	if hw.Spec.Metadata != nil {
		// BondingMode is valid even when 0 (balance-rr mode)
		// We check if metadata exists to distinguish "not set" from "set to 0"
		defaultMode = hw.Spec.Metadata.BondingMode
		if defaultMode < 0 || defaultMode > 6 {
			defaultMode = 1 // Invalid mode, fall back to active-backup
		}
	}

	// Group interfaces by bond name
	bondMembers := make(map[string][]v1alpha1.Interface)

	for _, iface := range hw.Spec.Interfaces {
		if iface.DHCP == nil || iface.DHCP.IfaceName == "" {
			continue
		}

		// Check if this is a bond member
		matches := bondInterfacePattern.FindStringSubmatch(iface.DHCP.IfaceName)
		if len(matches) != 3 {
			continue // Not a bond interface
		}

		bondName := fmt.Sprintf("bond%s", matches[1])
		bondMembers[bondName] = append(bondMembers[bondName], iface)
	}

	// Create bond configs from grouped members
	for bondName, members := range bondMembers {
		// Find the first member (phy0) which contains the configuration
		var firstMember *v1alpha1.Interface
		for i := range members {
			if members[i].DHCP != nil && members[i].DHCP.IfaceName != "" {
				matches := bondInterfacePattern.FindStringSubmatch(members[i].DHCP.IfaceName)
				if len(matches) == 3 && matches[2] == "0" {
					firstMember = &members[i]
					break
				}
			}
		}

		// If no phy0 found, use the first interface
		if firstMember == nil {
			firstMember = &members[0]
		}

		// Get bond mode: first try tags, then fall back to defaultMode
		mode := defaultMode
		if tagMode, ok := bondModes[bondName]; ok {
			mode = tagMode
		}

		// Extract IP and nameservers from first member
		var ip *v1alpha1.IP
		var nameservers []string
		if firstMember.DHCP != nil {
			ip = firstMember.DHCP.IP
			nameservers = firstMember.DHCP.NameServers
		}

		bonds[bondName] = &bondConfig{
			name:        bondName,
			mode:        mode,
			interfaces:  members,
			ip:          ip,
			nameservers: nameservers,
		}
	}

	return bonds
}

// parseUnbondedInterfaces returns interfaces not matching the bond pattern "bond<N>phy<M>".
// These interfaces will be configured independently with their own IP settings.
func parseUnbondedInterfaces(hw v1alpha1.Hardware) []v1alpha1.Interface {
	var unbonded []v1alpha1.Interface

	for _, iface := range hw.Spec.Interfaces {
		if iface.DHCP == nil || iface.DHCP.IfaceName == "" {
			continue
		}

		// Check if this is NOT a bond member
		if !bondInterfacePattern.MatchString(iface.DHCP.IfaceName) {
			unbonded = append(unbonded, iface)
		}
	}

	return unbonded
}

// generateNetworkConfigV2 creates a Network Config Version 2 (Netplan-compatible) from Hardware resource.
// Supports multiple bonds (bond<N>phy<M> pattern) and unbonded interfaces.
// Bond modes specified via tags (bond<N>:mode<M>) or Metadata.BondingMode.
// Returns nil if no interfaces are configured.
func generateNetworkConfigV2(hw v1alpha1.Hardware) *data.NetworkConfig {
	networkSpec := data.NetworkSpecV2{
		Version: 2,
	}

	// Parse bond configurations and unbonded interfaces
	bondConfigs := parseBondConfigurations(hw)
	unbondedInterfaces := parseUnbondedInterfaces(hw)

	// If no bonds and no unbonded interfaces with specific config, return nil
	if len(bondConfigs) == 0 && len(unbondedInterfaces) == 0 {
		return nil
	}

	ethernets := make(map[string]data.EthernetConfig)
	bonds := make(map[string]data.BondConfig)

	// Generate ethernet configs for bond members
	for bondName, bondCfg := range bondConfigs {
		bondInterfaces := []string{}

		for idx, iface := range bondCfg.interfaces {
			phyName := fmt.Sprintf("%sphy%d", bondName, idx)
			bondInterfaces = append(bondInterfaces, phyName)

			ethernets[phyName] = data.EthernetConfig{
				Dhcp4: false,
				Match: &data.MatchConfig{
					MACAddress: iface.DHCP.MAC,
				},
				SetName: phyName,
			}
		}

		// Create bond configuration
		bond := data.BondConfig{
			Interfaces: bondInterfaces,
			Parameters: generateBondParametersV2(bondCfg.mode),
		}

		// Add IP configuration to bond if present
		if bondCfg.ip != nil {
			family := inferIPFamily(bondCfg.ip.Family, bondCfg.ip.Address)

			addresses, gateway4, gateway6, _ := generateAddressConfigV2([]*v1alpha1.MetadataInstanceIP{
				{
					Address: bondCfg.ip.Address,
					Netmask: bondCfg.ip.Netmask,
					Gateway: bondCfg.ip.Gateway,
					Family:  family,
				},
			}, nil, nil)

			bond.Addresses = addresses
			bond.Gateway4 = gateway4
			bond.Gateway6 = gateway6
		} else {
			// Default to DHCP if no static IP
			bond.Dhcp4 = true
		}

		// Add nameservers if configured
		if len(bondCfg.nameservers) > 0 {
			bond.Nameservers = &data.NameserversConfig{
				Addresses: bondCfg.nameservers,
			}
		}

		bonds[bondName] = bond
	}

	// Generate ethernet configs for unbonded interfaces
	for _, iface := range unbondedInterfaces {
		ifaceName := iface.DHCP.IfaceName

		ethConfig := data.EthernetConfig{
			Match: &data.MatchConfig{
				MACAddress: iface.DHCP.MAC,
			},
			SetName: ifaceName,
		}

		// Configure IP if present
		if iface.DHCP.IP != nil {
			ethConfig.Dhcp4 = false
			family := inferIPFamily(iface.DHCP.IP.Family, iface.DHCP.IP.Address)

			addresses, gateway4, gateway6, _ := generateAddressConfigV2([]*v1alpha1.MetadataInstanceIP{
				{
					Address: iface.DHCP.IP.Address,
					Netmask: iface.DHCP.IP.Netmask,
					Gateway: iface.DHCP.IP.Gateway,
					Family:  family,
				},
			}, nil, nil)

			ethConfig.Addresses = addresses
			ethConfig.Gateway4 = gateway4
			ethConfig.Gateway6 = gateway6
		} else {
			// Default to DHCP
			ethConfig.Dhcp4 = true
		}

		// Add nameservers if configured
		if len(iface.DHCP.NameServers) > 0 {
			ethConfig.Nameservers = &data.NameserversConfig{
				Addresses: iface.DHCP.NameServers,
			}
		}

		ethernets[ifaceName] = ethConfig
	}

	networkSpec.Ethernets = ethernets
	networkSpec.Bonds = bonds

	return &data.NetworkConfig{
		Network: networkSpec,
	}
}


// generateBondParametersV2 creates bonding parameters (v2 format) based on bonding mode.
// V2 format uses hyphenated names without the "bond-" prefix.
func generateBondParametersV2(bondingMode int64) data.BondParameters {
	params := data.BondParameters{
		MIIMonitorInterval: 100,
	}

	switch bondingMode {
	case 0:
		params.Mode = "balance-rr"
	case 1:
		params.Mode = "active-backup"
		params.PrimaryReselectPolicy = "always"
		params.FailOverMACPolicy = "none"
	case 2:
		params.Mode = "balance-xor"
		params.TransmitHashPolicy = "layer2"
	case 3:
		params.Mode = "broadcast"
	case 4:
		params.Mode = "802.3ad"
		params.LACPRate = "fast"
		params.TransmitHashPolicy = "layer3+4"
		params.ADSelect = "stable"
	case 5:
		params.Mode = "balance-tlb"
	case 6:
		params.Mode = "balance-alb"
	default:
		// Default to active-backup for unknown modes
		params.Mode = "active-backup"
		params.PrimaryReselectPolicy = "always"
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
			// Convert netmask to CIDR prefix length
			var addr string
			if parsedIP := net.ParseIP(ip.Netmask); parsedIP != nil {
				if ipv4 := parsedIP.To4(); ipv4 != nil {
					ones, _ := net.IPMask(ipv4).Size()
					addr = fmt.Sprintf("%s/%d", ip.Address, ones)
				}
			}
			if addr != "" {
				addresses = append(addresses, addr)
			}
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
