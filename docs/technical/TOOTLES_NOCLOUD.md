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
        # First interface
    - dhcp:
        mac: b8:cb:29:98:cb:3b
        # Second interface
```

### Supported Bonding Modes

| Mode | Name | Description |
|------|------|-------------|
| 0 | balance-rr | Round-robin load balancing |
| 1 | active-backup | Active-backup fault tolerance |
| 2 | balance-xor | XOR load balancing |
| 3 | broadcast | Broadcast on all interfaces |
| 4 | 802.3ad | IEEE 802.3ad LACP (requires switch support) |
| 5 | balance-tlb | Adaptive transmit load balancing |
| 6 | balance-alb | Adaptive load balancing |

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
        dns_nameservers: [8.8.8.8, 8.8.4.4]
      - type: static6
        address: 2001:db8::10/64
        gateway: 2001:db8::1
        dns_nameservers: [2001:4860:4860::8888]
```

### Static IP Configuration

#### IPv4 Static IP

Specify IPv4 addresses with netmask:

```yaml
ips:
  - address: "192.168.1.10"
    netmask: "255.255.255.0"    # Converted to /24
    gateway: "192.168.1.1"
    family: 4
```

Supported netmasks:
- `255.255.255.0` → /24
- `255.255.255.240` → /28
- `255.255.255.192` → /26
- `255.255.0.0` → /16
- `255.0.0.0` → /8

#### IPv6 Static IP

Specify IPv6 addresses with CIDR prefix:

```yaml
ips:
  - address: "2001:db8::10/64"
    gateway: "2001:db8::1"
    family: 6
```

#### Dual-Stack Configuration

Configure both IPv4 and IPv6:

```yaml
ips:
  - address: "192.168.1.10"
    netmask: "255.255.255.0"
    gateway: "192.168.1.1"
    family: 4
  - address: "2001:db8::10/64"
    gateway: "2001:db8::1"
    family: 6
```

### Bond Parameters by Mode

#### Mode 4 (802.3ad LACP)

```yaml
params:
  bond-mode: 802.3ad
  bond-miimon: 100
  bond-lacp_rate: fast           # LACP packet rate (slow|fast)
  bond-xmit_hash_policy: layer3+4 # Load balancing hash policy
  bond-ad_select: stable          # Aggregation selection
```

#### Mode 1 (active-backup)

```yaml
params:
  bond-mode: active-backup
  bond-miimon: 100
  bond-primary_reselect: always
  bond-fail_over_mac: none
```

#### Mode 5 (balance-tlb)

```yaml
params:
  bond-mode: balance-tlb
  bond-miimon: 100
  bond-tlb_dynamic_lb: 1
```

### Cloud-init Integration

The NoCloud service is automatically available at the Tinkerbell service endpoint. Configure cloud-init to use it:

**PXE kernel parameters:**
```
ds=nocloud;seedfrom=http://<tinkerbell-ip>:7172/
```

**Or in cloud-init config:**
```yaml
# /etc/cloud/cloud.cfg.d/99-tinkerbell.cfg
datasource_list: [NoCloud]
datasource:
  NoCloud:
    seedfrom: http://<tinkerbell-ip>:7172/
```

## OS-Specific Notes

### Ubuntu 22.04 and 24.04

**Known Issue:** Canonical's cloud-init distributions include `no-nocloud-network.patch` which disables network-config from the NoCloud datasource.

**Workaround:** Revert the patch before cloud-init can use the network-config endpoint to create netplan configuration:

```bash
# Remove the patch from cloud-init
sudo rm /usr/lib/python3/dist-packages/cloudinit/sources/DataSourceNoCloud.py
sudo apt reinstall cloud-init
```

Or build a custom cloud-init package without the patch, or use an upstream cloud-init build.

### Rocky Linux 9.6

**Known Issue:** Cloud-init correctly parses the metadata and network-config, but the sysconfig renderer fails to create the bond interface properly.

**Workaround:** Manually apply the network configuration after cloud-init runs:

```bash
# Delete old DHCP connections
nmcli connection delete "System eth0" "System eth1" 2>/dev/null || true

# Restart NetworkManager to pick up generated config files
systemctl restart NetworkManager
```

Or use a post-provisioning script to ensure the bond is properly activated.

### Requirements

- cloud-init 24.4+ (for correct bond parameter naming)
- Network interfaces must support bonding
- For 802.3ad mode: Switch must support LACP
- At least 2 network interfaces defined in Hardware spec

### Verification

Verify the bond configuration after provisioning:

```bash
# Check bond status
cat /proc/net/bonding/bond0

# Verify IP configuration
ip addr show bond0

# Verify bonding mode and parameters
cat /sys/class/net/bond0/bonding/mode
cat /sys/class/net/bond0/bonding/slaves
```

### Troubleshooting

**Validate network-config schema:**
```bash
cloud-init schema --system
```

**Check cloud-init logs:**
```bash
tail -f /var/log/cloud-init.log
```

**Verify metadata service:**
```bash
curl http://<tinkerbell-ip>:7172/network-config
```

**Check for Ubuntu patch:**
```bash
grep -r "no-nocloud-network" /usr/lib/python3/dist-packages/cloudinit/
```

For more information on bond parameters, see the [Linux Kernel Bonding documentation](https://www.kernel.org/doc/Documentation/networking/bonding.txt).
