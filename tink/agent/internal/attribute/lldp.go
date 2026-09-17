package attribute

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

// DefaultLLDPTimeout is how long DiscoverLLDP waits for a neighbor
// advertisement, blocking agent startup for up to this long (see agent.go's
// ConfigureAndRun). This has to block rather than run in the background: a
// physical provisioning task can finish and power-cycle the machine well
// within that margin, killing discovery before it ever sees a neighbor if it
// isn't already done by the time the first action starts.
//
// In practice systemd-networkd starts well before the Agent (and before the
// Agent's own container image is even pulled) and keeps every advertisement
// it has seen, so almost every call returns near-instantly with data
// networkd already had. This bound only matters for a link that came up
// moments ago: one standard LLDP re-advertise interval (30s, most switches
// send an initial "fast start" burst on link-up well before that) plus
// margin for a missed cycle.
const DefaultLLDPTimeout = 35 * time.Second

// DiscoverLLDP returns the first neighbor systemd-networkd has seen on each
// interface, keyed by interface name, for up to timeout. Returns nil
// immediately if timeout is zero or less, or if networkd isn't usable (e.g.
// HookOS's LinuxKit base has no systemd/networkd at all) - callers must treat
// that as "no neighbors available" rather than a discovery failure.
func DiscoverLLDP(ctx context.Context, l logr.Logger, timeout time.Duration) map[string]*data.LLDPNeighbor {
	if timeout <= 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	neighbors, ok := discoverLLDPViaNetworkd(ctx, l)
	if !ok {
		l.V(1).Info("LLDP: systemd-networkd unavailable, skipping neighbor discovery")
		return nil
	}
	l.V(1).Info("LLDP: neighbors resolved via systemd-networkd", "neighbors", neighbors)
	return neighbors
}

// networkdPollInterval is how often discoverLLDPViaNetworkd re-queries
// networkctl while waiting for a neighbor, if the first query comes back
// empty (e.g. a link that came up moments ago and hasn't yet seen a
// re-advertisement). LLDP switches re-advertise roughly every 30s. Var
// rather than const so tests can shrink it instead of running at real time.
var networkdPollInterval = 2 * time.Second

// networkdQueryTimeout bounds a single `networkctl lldp` invocation - not the
// overall wait for a neighbor (that's discoverLLDPViaNetworkd's ctx), just
// protection against a hung or unresponsive networkd.
const networkdQueryTimeout = 5 * time.Second

// runNetworkctlLLDP runs `networkctl lldp --json=short` and returns its raw
// stdout. Var rather than a plain function so tests can replace it without
// depending on a running systemd-networkd.
var runNetworkctlLLDP = func(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, networkdQueryTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "networkctl", "lldp", "--json=short").Output()
}

// networkctlLLDPResponse mirrors the JSON shape of `networkctl lldp
// --json=short` (systemd's src/network/networkctl-lldp.c dispatch tables) -
// a systemd-defined schema, not a Tinkerbell one.
type networkctlLLDPResponse struct {
	Neighbors []networkctlLLDPInterface `json:"Neighbors"`
}

type networkctlLLDPInterface struct {
	InterfaceName string                   `json:"InterfaceName"`
	Neighbors     []networkctlLLDPNeighbor `json:"Neighbors"`
}

type networkctlLLDPNeighbor struct {
	ChassisID       string `json:"ChassisID"`
	PortID          string `json:"PortID"`
	PortDescription string `json:"PortDescription"`
	SystemName      string `json:"SystemName"`
	VlanID          uint32 `json:"VlanID"`
}

// queryNetworkctlLLDP runs and parses a single `networkctl lldp` call,
// keeping the first neighbor per interface. pending lists the interfaces
// networkd reported on that have no neighbor yet - callers use it to decide
// whether any of them are still worth waiting on. ok is false if networkctl
// couldn't be run at all (e.g. HookOS's LinuxKit base has no
// systemd/networkd) or its output couldn't be parsed - the caller must treat
// that as "no data available", not the same as a successful call that simply
// found no neighbors yet.
func queryNetworkctlLLDP(ctx context.Context, l logr.Logger) (neighbors map[string]*data.LLDPNeighbor, pending []string, ok bool) {
	out, err := runNetworkctlLLDP(ctx)
	if err != nil {
		l.V(1).Info("LLDP: networkctl unavailable", "error", err)
		return nil, nil, false
	}

	var resp networkctlLLDPResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		l.V(1).Info("LLDP: failed to parse networkctl output", "error", err)
		return nil, nil, false
	}

	neighbors = map[string]*data.LLDPNeighbor{}
	for _, iface := range resp.Neighbors {
		if len(iface.Neighbors) == 0 {
			pending = append(pending, iface.InterfaceName)
			continue
		}
		n := iface.Neighbors[0]
		neighbor := &data.LLDPNeighbor{
			ChassisID:       toPtr(n.ChassisID),
			PortID:          toPtr(n.PortID),
			PortDescription: toPtr(n.PortDescription),
			SystemName:      toPtr(n.SystemName),
		}
		if n.VlanID > 0 {
			neighbor.VLANIDs = []uint32{n.VlanID}
		}
		neighbors[iface.InterfaceName] = neighbor
	}
	return neighbors, pending, true
}

// hasCarrier reports whether the named interface currently has a live
// physical link, by reading /sys/class/net/<name>/carrier ("1" means carrier
// detected). Var rather than a plain function so tests can fake it without
// depending on real interfaces. Any error (interface administratively down,
// missing, or virtual) is treated as no carrier - the safe default, since it
// only ever makes discoverLLDPViaNetworkd stop waiting sooner, never wait
// longer than the timeout it already had.
var hasCarrier = func(name string) bool {
	b, err := os.ReadFile(filepath.Join("/sys/class/net", name, "carrier"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(b)) == "1"
}

// anyCarrier reports whether any of the named interfaces currently has a
// live link, i.e. whether it's still worth polling for an LLDP neighbor on
// at least one of them rather than concluding none will ever arrive - e.g.
// an interface with nothing plugged in will never get an LLDP neighbor no
// matter how long discoverLLDPViaNetworkd waits.
func anyCarrier(names []string) bool {
	for _, name := range names {
		if hasCarrier(name) {
			return true
		}
	}
	return false
}

// discoverLLDPViaNetworkd asks systemd-networkd for LLDP neighbors it has
// already accumulated, via `networkctl lldp --json=short`. networkd starts
// well before the Agent and keeps every advertisement it has seen, so this is
// normally instant; the poll loop only matters for a link that came up
// moments ago. It keeps polling as long as any interface still lacking a
// neighbor has a live link, rather than returning as soon as any single
// interface got one - the latter left slower interfaces (e.g. a link that
// came up a moment later) stuck with whatever they had after just one call.
// Interfaces with nothing plugged in are excluded from that wait, or a host
// with any down NIC would always block for the full timeout even though no
// neighbor was ever coming. ok is false if networkctl isn't usable at all -
// callers must not treat that as "zero neighbors".
func discoverLLDPViaNetworkd(ctx context.Context, l logr.Logger) (map[string]*data.LLDPNeighbor, bool) {
	for {
		neighbors, pending, ok := queryNetworkctlLLDP(ctx, l)
		if !ok || !anyCarrier(pending) {
			return neighbors, ok
		}
		select {
		case <-ctx.Done():
			return neighbors, true
		case <-time.After(networkdPollInterval):
		}
	}
}

// MergeLLDPNeighbors returns a copy of attrs with neighbors attached to the
// matching NetworkInterfaces, without mutating attrs or any of its nested
// values in place. Returns attrs unchanged if there's nothing to merge.
func MergeLLDPNeighbors(attrs *data.AgentAttributes, neighbors map[string]*data.LLDPNeighbor) *data.AgentAttributes {
	if attrs == nil || len(neighbors) == 0 {
		return attrs
	}
	merged := *attrs
	nics := make([]*data.Network, len(attrs.NetworkInterfaces))
	for i, nic := range attrs.NetworkInterfaces {
		if nic != nil && nic.Name != nil {
			if n, ok := neighbors[*nic.Name]; ok {
				withNeighbor := *nic
				withNeighbor.LLDPNeighbor = n
				nics[i] = &withNeighbor
				continue
			}
		}
		nics[i] = nic
	}
	merged.NetworkInterfaces = nics
	return &merged
}
