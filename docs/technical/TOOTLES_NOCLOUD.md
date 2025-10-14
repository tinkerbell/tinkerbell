# Tootles - NoCloud Network Bonding

The NoCloud metadata service in Tinkerbell provides network bonding configuration for bare metal servers with static IP addresses, enabling cloud-init to configure bonded interfaces during provisioning.

## Network Config Format

Tinkerbell generates Network Config Version 2 (Netplan-compatible) format for all hardware resources.

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
    - dhcp:
        mac: b8:cb:29:98:cb:3b
```

### Generated Network Configuration

The service generates Network Config Version 2 (Netplan-compatible format):

```yaml
network:
  version: 2
  ethernets:
    bond0phy0:
      match:
        macaddress: b8:cb:29:98:cb:3a
      set-name: bond0phy0
      dhcp4: false
    bond0phy1:
      match:
        macaddress: b8:cb:29:98:cb:3b
      set-name: bond0phy1
      dhcp4: false
  bonds:
    bond0:
      interfaces: [bond0phy0, bond0phy1]
      parameters:
        mode: 802.3ad
        mii-monitor-interval: 100
        lacp-rate: fast
        transmit-hash-policy: layer3+4
        ad-select: stable
      addresses:
        - 192.168.1.10/24
        - 2001:db8::10/64
      gateway4: 192.168.1.1
      gateway6: 2001:db8::1
      nameservers:
        addresses: [1.1.1.1, 1.0.0.1, 2606:4700:4700::1111]
```

**Note:** Physical interfaces are matched by MAC address and renamed to `bond0phyX` (where X is the interface index) using `set-name`. This ensures consistent interface naming across reboots.


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

- cloud-init 24.4+ recommended
- Network interfaces must support bonding
- For 802.3ad mode: Switch must support LACP
- At least 2 network interfaces defined in Hardware spec
- Ubuntu 17.10+, modern Debian/RHEL with Netplan or systemd-networkd
