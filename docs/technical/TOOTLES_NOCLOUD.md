# Tootles - NoCloud Network Bonding

The NoCloud metadata service in Tinkerbell provides network bonding configuration for bare metal servers with static IP addresses, enabling cloud-init to configure bonded interfaces during provisioning.

## Network Bonding with Static IPs

### Hardware Configuration

Configure bonding and static IPs in your Hardware resource:

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: server-001
spec:
  metadata:
    bonding_mode: 4  # 802.3ad LACP
    instance:
      hostname: "server001.example.com"
      id: "b8:cb:29:98:cb:3a"
      ips:
        - address: "192.168.1.10"
          netmask: "255.255.255.0"
          gateway: "192.168.1.1"
          family: 4
        - address: "2001:db8::10/64"
          gateway: "2001:db8::1"
          family: 6
  interfaces:
    - dhcp:
        mac: b8:cb:29:98:cb:3a
        name_servers:
          - "1.1.1.1"
          - "1.0.0.1"
          - "2606:4700:4700::1111"
        # First interface
    - dhcp:
        mac: b8:cb:29:98:cb:3b
        # Second interface
```

### Generated Network Configuration

The NoCloud service automatically generates network-config for cloud-init:

```yaml
version: 1
config:
  - type: physical
    name: eno1
    mac_address: b8:cb:29:98:cb:3a
    mtu: 1500

  - type: physical
    name: eno2
    mac_address: b8:cb:29:98:cb:3b
    mtu: 1500

  - type: bond
    name: bond0
    bond_interfaces: [eno1, eno2]
    mtu: 1500
    params:
      bond-mode: 802.3ad
      bond-miimon: 100
      bond-lacp_rate: fast
      bond-xmit_hash_policy: layer3+4
      bond-ad_select: stable
    subnets:
      - type: static
        address: 192.168.1.10/24
        gateway: 192.168.1.1
        dns_nameservers: [1.1.1.1, 1.0.0.1]
      - type: static6
        address: 2001:db8::10/64
        gateway: 2001:db8::1
        dns_nameservers: [2606:4700:4700::1111]
```


### Cloud-init Integration

The NoCloud service is automatically available at the Tinkerbell service endpoint. Configure cloud-init to use it:

**PXE kernel parameters:**
```
ds=nocloud;seedfrom=http://<tinkerbell-ip>:7172/
```


## OS-Specific Notes

### Ubuntu 22.04 and 24.04

**Known Issue:** Canonical's cloud-init distributions include `no-nocloud-network.patch` which disables network-config from the NoCloud datasource.

**Workaround:** Revert the patch before cloud-init can use the network-config endpoint to create netplan configuration:

### Requirements

- cloud-init 24.4+ (for correct bond parameter naming)
- Network interfaces must support bonding
- For 802.3ad mode: Switch must support LACP
- At least 2 network interfaces defined in Hardware spec
