package containerd

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const (
	// resolvConfPath is the default host resolv.conf path.
	resolvConfPath = "/etc/resolv.conf"
	// systemdResolvedPath is the systemd-resolved upstream resolv.conf.
	// When the host uses systemd-resolved, its /etc/resolv.conf points to
	// 127.0.0.53 which is not reachable from a container network namespace.
	// In that case we read the upstream resolv.conf instead.
	systemdResolvedPath = "/run/systemd/resolve/resolv.conf"
)

// fallbackNameservers are used when the host has no reachable nameservers
// (e.g. all are localhost and we're in an isolated network namespace, or
// /etc/resolv.conf is unreadable). Google Public DNS, IPv4 and IPv6.
var fallbackNameservers = []string{"8.8.8.8", "8.8.4.4", "2001:4860:4860::8888", "2001:4860:4860::8844"}

// truncateHostname returns the first 12 characters of id, matching the
// nerdctl/Docker convention for deriving a short hostname from a container ID.
func truncateHostname(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// dnsFiles holds paths to generated DNS configuration files that should be
// bind-mounted into a container. The caller is responsible for cleaning up
// the directory after the container exits.
type dnsFiles struct {
	dir        string
	resolvConf string
	hosts      string
	hostname   string
}

// cleanup removes all generated DNS files.
func (d *dnsFiles) cleanup() error {
	if d == nil || d.dir == "" {
		return nil
	}
	return os.RemoveAll(d.dir)
}

// prepareDNSFiles creates the temp directory and writes resolv.conf. The hosts
// and hostname files are created as empty placeholders so the bind-mount paths
// exist at container creation time. Call setHostname after the container is
// created to populate them with the container's hostname.
//
// allowLocalhostDNS should be true for host-network containers where
// localhost resolvers (e.g. systemd-resolved on 127.0.0.53) are reachable.
func prepareDNSFiles(allowLocalhostDNS bool) (*dnsFiles, error) {
	dir, err := os.MkdirTemp("", "tink-dns-")
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS temp dir: %w", err)
	}

	df := &dnsFiles{dir: dir}

	// Build resolv.conf — this doesn't depend on the container ID.
	df.resolvConf = filepath.Join(dir, "resolv.conf")
	if err := buildResolvConf(df.resolvConf, allowLocalhostDNS); err != nil {
		_ = df.cleanup()
		return nil, fmt.Errorf("failed to build resolv.conf: %w", err)
	}

	// Create empty placeholder files for hosts and hostname so the bind-mount
	// source paths exist when the container spec is applied. The actual content
	// is written by setHostname() after the container is created.
	df.hosts = filepath.Join(dir, "hosts")
	if err := os.WriteFile(df.hosts, []byte{}, 0o644); err != nil { //nolint:gosec // G306: DNS files must be world-readable inside the container
		_ = df.cleanup()
		return nil, fmt.Errorf("failed to create hosts placeholder: %w", err)
	}

	df.hostname = filepath.Join(dir, "hostname")
	if err := os.WriteFile(df.hostname, []byte{}, 0o644); err != nil { //nolint:gosec // G306: DNS files must be world-readable inside the container
		_ = df.cleanup()
		return nil, fmt.Errorf("failed to create hostname placeholder: %w", err)
	}

	return df, nil
}

// setHostname populates the hosts and hostname files. For host-network
// containers (useHostNetwork=true), the full hostname is used without
// truncation and the host's /etc/hosts is copied so the container behaves
// like a host process. For isolated-network containers, the hostname is
// truncated to 12 characters (matching nerdctl/Docker convention) and a
// minimal hosts file is generated.
func (d *dnsFiles) setHostname(name string, useHostNetwork bool) error {
	if d == nil {
		return nil
	}

	if useHostNetwork {
		return d.setHostnameHost(name)
	}
	return d.setHostnameIsolated(name)
}

// setHostnameHost populates DNS files for host-network containers. The host's
// /etc/hosts is copied directly and the full hostname is used without truncation.
func (d *dnsFiles) setHostnameHost(name string) error {
	// Copy the host's /etc/hosts so the container sees the same entries.
	hostContent, err := os.ReadFile("/etc/hosts")
	if err != nil {
		// Fall back to generating a hosts file if we can't read the host's.
		if err := buildHosts(d.hosts, name); err != nil {
			return fmt.Errorf("failed to write hosts: %w", err)
		}
	} else {
		if err := os.WriteFile(d.hosts, hostContent, 0o644); err != nil { //nolint:gosec // G306: DNS files must be world-readable inside the container
			return fmt.Errorf("failed to copy host /etc/hosts: %w", err)
		}
	}
	// Use the full hostname without truncation.
	if err := os.WriteFile(d.hostname, []byte(name+"\n"), 0o644); err != nil { //nolint:gosec // G306: DNS files must be world-readable inside the container
		return fmt.Errorf("failed to write hostname: %w", err)
	}
	return nil
}

// setHostnameIsolated populates DNS files for isolated-network containers.
// The hostname is truncated to 12 characters (matching nerdctl/Docker convention).
func (d *dnsFiles) setHostnameIsolated(name string) error {
	if err := buildHosts(d.hosts, truncateHostname(name)); err != nil {
		return fmt.Errorf("failed to write hosts: %w", err)
	}
	if err := os.WriteFile(d.hostname, []byte(truncateHostname(name)+"\n"), 0o644); err != nil { //nolint:gosec // G306: DNS files must be world-readable inside the container
		return fmt.Errorf("failed to write hostname: %w", err)
	}
	return nil
}

// dnsSpecOpts returns OCI spec options that bind-mount the generated DNS
// files into the container.
func dnsSpecOpts(df *dnsFiles) []oci.SpecOpts {
	return []oci.SpecOpts{
		withBindMount(df.resolvConf, "/etc/resolv.conf"),
		withBindMount(df.hosts, "/etc/hosts"),
		withBindMount(df.hostname, "/etc/hostname"),
	}
}

// withBindMount returns an OCI spec option that adds a bind mount.
func withBindMount(src, dest string) oci.SpecOpts {
	return oci.WithMounts([]specs.Mount{
		{
			Destination: dest,
			Type:        "bind",
			Source:      src,
			Options:     []string{"bind", "rprivate", "rw"},
		},
	})
}

// buildResolvConf reads the host's resolv.conf and writes a container-safe
// resolv.conf to dst. When allowLocalhostDNS is false, localhost nameservers
// are filtered out (they are unreachable from an isolated network namespace)
// and public DNS fallbacks are substituted if no nameservers remain.
// When allowLocalhostDNS is true (host-network mode), localhost nameservers
// are preserved since they are reachable from the host network namespace.
func buildResolvConf(dst string, allowLocalhostDNS bool) error {
	hostResolvConf := resolvConfPath

	// Detect systemd-resolved: if /etc/resolv.conf points to 127.0.0.53,
	// and we are filtering localhost, read the upstream config instead.
	if !allowLocalhostDNS {
		if content, err := os.ReadFile(resolvConfPath); err == nil {
			for _, ns := range parseNameservers(string(content)) {
				if ns == "127.0.0.53" {
					if _, err := os.Stat(systemdResolvedPath); err == nil {
						hostResolvConf = systemdResolvedPath
					}
					break
				}
			}
		}
	}

	content, err := os.ReadFile(hostResolvConf)
	if err != nil {
		// If we can't read any resolv.conf, write a minimal one with public DNS.
		return writeResolvConf(dst, fallbackNameservers, nil, nil)
	}

	nameservers, searchDomains, options := parseResolvConf(string(content))

	if !allowLocalhostDNS {
		// Filter out localhost nameservers — they are unreachable from a
		// container's isolated network namespace.
		var filtered []string
		for _, ns := range nameservers {
			if isLocalhost(ns) {
				continue
			}
			filtered = append(filtered, ns)
		}

		// If all nameservers were localhost, fall back to public DNS.
		if len(filtered) == 0 {
			filtered = fallbackNameservers
		}
		nameservers = filtered
	}

	return writeResolvConf(dst, nameservers, searchDomains, options)
}

// parseResolvConf extracts nameservers, search domains, and options from
// resolv.conf content.
func parseResolvConf(content string) (nameservers, searchDomains, options []string) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "nameserver":
			nameservers = append(nameservers, fields[1])
		case "search":
			searchDomains = append(searchDomains, fields[1:]...)
		case "options":
			options = append(options, fields[1:]...)
		}
	}
	return nameservers, searchDomains, options
}

// parseNameservers is a quick helper that extracts just the nameservers.
func parseNameservers(content string) []string {
	ns, _, _ := parseResolvConf(content)
	return ns
}

// isLocalhost returns true if the IP string is a loopback address.
func isLocalhost(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}

// writeResolvConf writes a resolv.conf file with the given parameters.
func writeResolvConf(path string, nameservers, searchDomains, options []string) error {
	var b strings.Builder
	b.WriteString("# Generated by tink-agent for container DNS\n")
	if len(searchDomains) > 0 {
		b.WriteString("search " + strings.Join(searchDomains, " ") + "\n")
	}
	for _, ns := range nameservers {
		b.WriteString("nameserver " + ns + "\n")
	}
	if len(options) > 0 {
		b.WriteString("options " + strings.Join(options, " ") + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644) //nolint:gosec // G306: resolv.conf must be world-readable inside the container
}

// buildHosts writes a minimal /etc/hosts with localhost entries and the
// given hostname mapped to its loopback address. The caller is responsible
// for any hostname truncation before calling this function.
func buildHosts(dst, hostname string) error {
	var b strings.Builder
	b.WriteString("# Generated by tink-agent\n")
	b.WriteString("127.0.0.1\tlocalhost\n")
	b.WriteString("::1\t\tlocalhost ip6-localhost ip6-loopback\n")
	b.WriteString("fe00::0\t\tip6-localnet\n")
	b.WriteString("ff00::0\t\tip6-mcastprefix\n")
	b.WriteString("ff02::1\t\tip6-allnodes\n")
	b.WriteString("ff02::2\t\tip6-allrouters\n")
	b.WriteString("127.0.0.1\t" + hostname + "\n")
	b.WriteString("::1\t\t" + hostname + "\n")
	return os.WriteFile(dst, []byte(b.String()), 0o644) //nolint:gosec // G306: hosts file must be world-readable inside the container
}
