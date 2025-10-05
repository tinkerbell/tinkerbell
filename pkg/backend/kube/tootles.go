package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

	return toNoCloudInstance(*hw), nil
}

// toNoCloudInstance converts a Tinkerbell Hardware resource to a NoCloudInstance.
func toNoCloudInstance(hw v1alpha1.Hardware) data.NoCloudInstance {
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
	i.NetworkConfig = generateNetworkConfig(hw)

	return i
}

// generateNetworkConfig creates a NoCloud-compatible network configuration from Hardware resource.
func generateNetworkConfig(hw v1alpha1.Hardware) interface{} {
	config := map[string]interface{}{
		"version": 1,
		"config":  []interface{}{},
	}

	configSlice := []interface{}{}

	// Check if bonding is enabled
	bondingEnabled := hw.Spec.Metadata != nil && hw.Spec.Metadata.BondingMode > 0

	switch {
	case bondingEnabled && len(hw.Spec.Interfaces) >= 2:
		// Generate bonding configuration
		configSlice = generateBondingConfiguration(hw)
	case len(hw.Spec.Interfaces) == 0:
		// No interfaces defined - generate fallback configuration
		if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil && len(hw.Spec.Metadata.Instance.Ips) > 0 {
			configSlice = append(configSlice, generatePhysicalInterfaceWithIPs(hw.Spec.Metadata.Instance.Ips))
		} else {
			// Fallback to basic DHCP configuration
			configSlice = append(configSlice, map[string]interface{}{
				"type": "physical",
				"name": "eno1",
				"subnets": []interface{}{
					map[string]interface{}{
						"type": "dhcp",
					},
				},
			})
		}
	default:
		// Generate standard physical interface configuration
		configSlice = generatePhysicalInterfaceConfiguration(hw)
	}

	// Add DNS configuration
	dnsConfig := map[string]interface{}{
		"type": "nameserver",
		"address": []string{
			"8.8.8.8",
			"8.8.4.4",
			"2001:4860:4860::8888",
			"2001:4860:4860::8844",
		},
	}
	configSlice = append(configSlice, dnsConfig)

	config["config"] = configSlice
	return config
}

// generateBondingConfiguration creates bonding configuration from Hardware resource.
func generateBondingConfiguration(hw v1alpha1.Hardware) []interface{} {
	configSlice := []interface{}{}
	bondInterfaces := []string{}

	// Create physical interfaces for bonding (without subnets)
	for i, iface := range hw.Spec.Interfaces {
		interfaceName := fmt.Sprintf("eno%d", i+1)
		bondInterfaces = append(bondInterfaces, interfaceName)

		physicalConfig := map[string]interface{}{
			"type": "physical",
			"name": interfaceName,
			"mtu":  1500,
		}

		// Add MAC address if available in DHCP config
		if iface.DHCP != nil && iface.DHCP.MAC != "" {
			physicalConfig["mac_address"] = iface.DHCP.MAC
		}

		configSlice = append(configSlice, physicalConfig)
	}

	// Create bond configuration
	bondConfig := map[string]interface{}{
		"type":            "bond",
		"name":            "bond0",
		"bond_interfaces": bondInterfaces,
		"mtu":             1500,
		"params":          generateBondParameters(hw.Spec.Metadata.BondingMode),
	}

	// Add IP configuration to bond
	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil && len(hw.Spec.Metadata.Instance.Ips) > 0 {
		bondConfig["subnets"] = generateSubnetsFromIPs(hw.Spec.Metadata.Instance.Ips)
	} else {
		// Default to DHCP if no static IPs
		bondConfig["subnets"] = []interface{}{
			map[string]interface{}{"type": "dhcp"},
			map[string]interface{}{"type": "dhcp6"},
		}
	}

	configSlice = append(configSlice, bondConfig)
	return configSlice
}

// generatePhysicalInterfaceConfiguration creates standard physical interface configuration.
func generatePhysicalInterfaceConfiguration(hw v1alpha1.Hardware) []interface{} {
	configSlice := []interface{}{}

	for i, iface := range hw.Spec.Interfaces {
		interfaceName := fmt.Sprintf("eno%d", i+1)

		physicalConfig := map[string]interface{}{
			"type": "physical",
			"name": interfaceName,
			"mtu":  1500,
		}

		// Add MAC address if available
		if iface.DHCP != nil && iface.DHCP.MAC != "" {
			physicalConfig["mac_address"] = iface.DHCP.MAC
		}

		// Add subnets configuration based on interface settings
		subnets := []interface{}{}

		if !iface.DisableDHCP {
			subnets = append(subnets, map[string]interface{}{
				"type": "dhcp",
			})
			subnets = append(subnets, map[string]interface{}{
				"type": "dhcp6",
			})
		}

		if len(subnets) > 0 {
			physicalConfig["subnets"] = subnets
		}

		configSlice = append(configSlice, physicalConfig)
	}

	// If we have metadata IPs, try to create static configuration for the first interface
	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil && len(hw.Spec.Metadata.Instance.Ips) > 0 {
		staticConfig := generateStaticNetworkConfig(hw.Spec.Metadata.Instance.Ips)
		if staticConfig != nil {
			// Replace the first interface with static configuration
			if len(configSlice) > 0 {
				configSlice[0] = staticConfig
			}
		}
	}

	return configSlice
}

// generateBondParameters creates bonding parameters based on bonding mode.
func generateBondParameters(bondingMode int64) map[string]interface{} {
	params := map[string]interface{}{
		"bond-miimon": 100,
	}

	switch bondingMode {
	case 0:
		params["bond-mode"] = "balance-rr"
		params["bond-use_carrier"] = 1
	case 1:
		params["bond-mode"] = "active-backup"
		params["bond-primary_reselect"] = "always"
		params["bond-fail_over_mac"] = "none"
	case 2:
		params["bond-mode"] = "balance-xor"
		params["bond-xmit_hash_policy"] = "layer2"
	case 3:
		params["bond-mode"] = "broadcast"
	case 4:
		params["bond-mode"] = "802.3ad"
		params["bond-lacp_rate"] = "fast"
		params["bond-xmit_hash_policy"] = "layer3+4"
		params["bond-ad_select"] = "stable"
	case 5:
		params["bond-mode"] = "balance-tlb"
		params["bond-tlb_dynamic_lb"] = 1
	case 6:
		params["bond-mode"] = "balance-alb"
		params["bond-rlb_update_delay"] = 0
	default:
		// Default to active-backup for unknown modes
		params["bond-mode"] = "active-backup"
		params["bond-primary_reselect"] = "always"
	}

	return params
}

// generateSubnetsFromIPs creates subnet configuration from IP metadata.
func generateSubnetsFromIPs(ips []*v1alpha1.MetadataInstanceIP) []interface{} {
	subnets := []interface{}{}

	for _, ip := range ips {
		switch ip.Family {
		case 4:
			subnet := map[string]interface{}{
				"type":    "static",
				"address": fmt.Sprintf("%s/%s", ip.Address, cidrFromNetmask(ip.Netmask)),
			}
			if ip.Gateway != "" {
				subnet["gateway"] = ip.Gateway
			}
			subnet["dns_nameservers"] = []string{"8.8.8.8", "8.8.4.4"}
			subnets = append(subnets, subnet)
		case 6:
			subnet := map[string]interface{}{
				"type":    "static6",
				"address": ip.Address,
			}
			if ip.Gateway != "" {
				subnet["gateway"] = ip.Gateway
			}
			subnet["dns_nameservers"] = []string{"2001:4860:4860::8888"}
			subnets = append(subnets, subnet)
		}
	}

	// If no static IPs, fall back to DHCP
	if len(subnets) == 0 {
		subnets = append(subnets, map[string]interface{}{"type": "dhcp"})
		subnets = append(subnets, map[string]interface{}{"type": "dhcp6"})
	}

	return subnets
}

// generatePhysicalInterfaceWithIPs creates a basic physical interface with IP configuration.
func generatePhysicalInterfaceWithIPs(ips []*v1alpha1.MetadataInstanceIP) map[string]interface{} {
	physicalConfig := map[string]interface{}{
		"type": "physical",
		"name": "eno1",
		"mtu":  1500,
	}

	subnets := []interface{}{}

	// Add static IP configurations
	for _, ip := range ips {
		switch ip.Family {
		case 4:
			subnet := map[string]interface{}{
				"type":    "static",
				"address": fmt.Sprintf("%s/%s", ip.Address, cidrFromNetmask(ip.Netmask)),
			}
			if ip.Gateway != "" {
				subnet["gateway"] = ip.Gateway
			}
			subnet["dns_nameservers"] = []string{"8.8.8.8", "8.8.4.4"}
			subnets = append(subnets, subnet)
		case 6:
			subnet := map[string]interface{}{
				"type":    "static6",
				"address": ip.Address, // IPv6 addresses should already include prefix
			}
			if ip.Gateway != "" {
				subnet["gateway"] = ip.Gateway
			}
			subnet["dns_nameservers"] = []string{"2001:4860:4860::8888"}
			subnets = append(subnets, subnet)
		}
	}

	// If no static IPs, fall back to DHCP
	if len(subnets) == 0 {
		subnets = append(subnets, map[string]interface{}{
			"type": "dhcp",
		})
	}

	physicalConfig["subnets"] = subnets
	return physicalConfig
}

// generateStaticNetworkConfig creates static network configuration from metadata IPs.
func generateStaticNetworkConfig(ips []*v1alpha1.MetadataInstanceIP) map[string]interface{} {
	if len(ips) == 0 {
		return nil
	}

	config := map[string]interface{}{
		"type": "physical",
		"name": "eno1",
		"mtu":  1500,
	}

	subnets := []interface{}{}

	for _, ip := range ips {
		switch ip.Family {
		case 4:
			subnet := map[string]interface{}{
				"type":    "static",
				"address": fmt.Sprintf("%s/%s", ip.Address, cidrFromNetmask(ip.Netmask)),
			}
			if ip.Gateway != "" {
				subnet["gateway"] = ip.Gateway
			}
			subnet["dns_nameservers"] = []string{"8.8.8.8", "8.8.4.4"}
			subnets = append(subnets, subnet)
		case 6:
			subnet := map[string]interface{}{
				"type":    "static6",
				"address": ip.Address,
			}
			if ip.Gateway != "" {
				subnet["gateway"] = ip.Gateway
			}
			subnet["dns_nameservers"] = []string{"2001:4860:4860::8888"}
			subnets = append(subnets, subnet)
		}
	}

	if len(subnets) > 0 {
		config["subnets"] = subnets
		return config
	}

	return nil
}

// cidrFromNetmask converts a netmask to CIDR notation.
// This is a simple implementation that handles common netmasks.
func cidrFromNetmask(netmask string) string {
	switch netmask {
	case "255.255.255.0":
		return "24"
	case "255.255.0.0":
		return "16"
	case "255.0.0.0":
		return "8"
	case "255.255.255.192":
		return "26"
	case "255.255.255.224":
		return "27"
	case "255.255.255.240":
		return "28"
	case "255.255.255.248":
		return "29"
	case "255.255.255.252":
		return "30"
	default:
		// Default to /24 if we can't determine the CIDR
		return "24"
	}
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
