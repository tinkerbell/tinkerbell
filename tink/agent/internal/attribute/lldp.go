package attribute

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

// networkctlTimeout bounds a single networkctl call, so a hung networkd
// can't consume the whole LLDP wait.
const networkctlTimeout = 5 * time.Second

// lldp discovers LLDP neighbors from the table systemd-networkd keeps.
type lldp struct {
	// networkctl returns the stdout of `networkctl lldp --json=short`.
	networkctl func(ctx context.Context) ([]byte, error)
	// sysClassNet is the sysfs directory interface carrier state is read from.
	sysClassNet  string
	pollInterval time.Duration
}

func newLLDP() lldp {
	return lldp{
		networkctl:   runNetworkctlLLDP,
		sysClassNet:  "/sys/class/net",
		pollInterval: 2 * time.Second,
	}
}

// neighbors returns the LLDP neighbors of ifaces, keyed by interface name.
// It waits up to timeout for every interface in ifaces that has a link to
// report at least one neighbor. It returns nil if timeout is zero or less,
// or if networkctl is unavailable, as on HookOS which has no systemd.
func (d lldp) neighbors(ctx context.Context, l logr.Logger, timeout time.Duration, ifaces []string) map[string][]*data.LLDPNeighbor {
	if timeout <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var found map[string][]*data.LLDPNeighbor
	for {
		n, ok := d.query(ctx, l)
		if !ok {
			// Keep what an earlier query found rather than losing it to a later failure.
			return found
		}
		found = n

		// networkctl omits an interface until it has a neighbor, so ifaces,
		// not the response, says what is still pending. An interface without
		// a link will never get one, so it isn't worth waiting on.
		if !d.anyCarrier(pending(ifaces, found)) {
			return found
		}
		select {
		case <-ctx.Done():
			return found
		case <-time.After(d.pollInterval):
		}
	}
}

// query reports the neighbors networkd currently has for each interface,
// sorted by chassis ID then port ID. ok is false if networkctl could not be
// run or its output could not be parsed.
func (d lldp) query(ctx context.Context, l logr.Logger) (neighbors map[string][]*data.LLDPNeighbor, ok bool) {
	out, err := d.networkctl(ctx)
	if err != nil {
		switch {
		case errors.Is(err, exec.ErrNotFound):
			l.V(1).Info("LLDP: networkctl not found, skipping neighbor discovery")
		case ctx.Err() != nil:
			l.V(1).Info("LLDP: stopped waiting for networkctl", "error", ctx.Err())
		default:
			l.Info("LLDP: networkctl lldp failed, skipping neighbor discovery", "error", err.Error())
		}
		return nil, false
	}

	var resp networkctlLLDPResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		l.Info("LLDP: could not parse the networkctl lldp output, skipping neighbor discovery", "error", err.Error())
		return nil, false
	}

	neighbors = map[string][]*data.LLDPNeighbor{}
	for _, iface := range resp.Neighbors {
		slices.SortFunc(iface.Neighbors, func(a, b networkctlLLDPNeighbor) int {
			return cmp.Or(cmp.Compare(a.ChassisID, b.ChassisID), cmp.Compare(a.PortID, b.PortID))
		})
		for _, n := range iface.Neighbors {
			neighbors[iface.InterfaceName] = append(neighbors[iface.InterfaceName], &data.LLDPNeighbor{
				ChassisID:       nonEmpty(n.ChassisID),
				PortID:          nonEmpty(n.PortID),
				PortDescription: nonEmpty(n.PortDescription),
				SystemName:      nonEmpty(n.SystemName),
			})
		}
	}
	return neighbors, true
}

// anyCarrier reports whether any of the named interfaces has a link.
func (d lldp) anyCarrier(names []string) bool {
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(d.sysClassNet, name, "carrier"))
		if err == nil && strings.TrimSpace(string(b)) == "1" {
			return true
		}
	}
	return false
}

// pending returns the names in ifaces that have no entry in found.
func pending(ifaces []string, found map[string][]*data.LLDPNeighbor) []string {
	var names []string
	for _, name := range ifaces {
		if _, ok := found[name]; !ok {
			names = append(names, name)
		}
	}
	return names
}

func runNetworkctlLLDP(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, networkctlTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "networkctl", "lldp", "--json=short").Output()
	// Output keeps networkctl's explanation of a failure only on the ExitError.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			err = fmt.Errorf("%w (stderr: %s)", err, stderr)
		}
	}
	return out, err
}

// networkctlLLDPResponse is the systemd-defined JSON shape of
// `networkctl lldp --json=short`.
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
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
