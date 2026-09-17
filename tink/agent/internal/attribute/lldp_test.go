package attribute

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

func TestMergeLLDPNeighbors(t *testing.T) {
	name := func(s string) *string { return &s }
	eno1 := &data.Network{Name: name("eno1")}
	eno2 := &data.Network{Name: name("eno2")}
	attrs := &data.AgentAttributes{NetworkInterfaces: []*data.Network{eno1, eno2}}

	neighbor := &data.LLDPNeighbor{SystemName: name("switch01")}
	merged := MergeLLDPNeighbors(attrs, map[string]*data.LLDPNeighbor{"eno1": neighbor})

	if merged == attrs {
		t.Fatal("MergeLLDPNeighbors returned the same pointer, want a new value")
	}
	if len(merged.NetworkInterfaces) != 2 {
		t.Fatalf("NetworkInterfaces = %+v, want two entries", merged.NetworkInterfaces)
	}
	if merged.NetworkInterfaces[0] == eno1 {
		t.Error("the merged eno1 entry shares the original pointer, want a copy since it was modified")
	}
	if merged.NetworkInterfaces[0].LLDPNeighbor != neighbor {
		t.Errorf("merged eno1 LLDPNeighbor = %+v, want %+v", merged.NetworkInterfaces[0].LLDPNeighbor, neighbor)
	}
	if merged.NetworkInterfaces[1] != eno2 {
		t.Error("the merged eno2 entry (no neighbor found) should share the original pointer unchanged")
	}
	// The original must be untouched - a concurrent reader may still hold it.
	if eno1.LLDPNeighbor != nil {
		t.Errorf("original eno1.LLDPNeighbor = %+v, want nil (must not mutate the input)", eno1.LLDPNeighbor)
	}
}

func TestMergeLLDPNeighborsNoMatch(t *testing.T) {
	attrs := &data.AgentAttributes{}
	if got := MergeLLDPNeighbors(attrs, nil); got != attrs {
		t.Errorf("MergeLLDPNeighbors with no neighbors = %v, want the same attrs pointer unchanged", got)
	}
	if got := MergeLLDPNeighbors(nil, map[string]*data.LLDPNeighbor{"eno1": {}}); got != nil {
		t.Errorf("MergeLLDPNeighbors(nil, ...) = %v, want nil", got)
	}
}

func TestDiscoverLLDPDisabledByDefault(t *testing.T) {
	if got := DiscoverLLDP(context.Background(), logr.Discard(), 0); got != nil {
		t.Errorf("DiscoverLLDP with zero timeout = %v, want nil", got)
	}
}

// withNetworkctlLLDP replaces runNetworkctlLLDP for the duration of the test.
func withNetworkctlLLDP(t *testing.T, fn func(ctx context.Context) ([]byte, error)) {
	t.Helper()
	orig := runNetworkctlLLDP
	runNetworkctlLLDP = fn
	t.Cleanup(func() { runNetworkctlLLDP = orig })
}

// withHasCarrier replaces hasCarrier for the duration of the test, so tests
// don't depend on real interfaces existing on the machine running them.
func withHasCarrier(t *testing.T, fn func(name string) bool) {
	t.Helper()
	orig := hasCarrier
	hasCarrier = fn
	t.Cleanup(func() { hasCarrier = orig })
}

// networkctlSample is real output shape from `networkctl lldp --json=short`
// (github.com/tinkerbell/tinkerbell/pull/944#issuecomment-5424795070),
// trimmed to the fields queryNetworkctlLLDP reads.
const networkctlSample = `{"Neighbors":[{"InterfaceIndex":4,"InterfaceName":"eno1","Neighbors":[{"ChassisID":"16:ae:d6:24:62:9f","PortID":"d0:11:e5:1c:6f:d8","PortDescription":"en0","SystemName":"switch01.example.com","SystemDescription":"desc","EnabledCapabilities":12,"VlanID":10},{"ChassisID":"ignored-second-neighbor","PortID":"ignored","SystemName":"ignored"}]},{"InterfaceIndex":5,"InterfaceName":"eno2","Neighbors":[]}]}`

// networkctlSampleAllFound is networkctlSample with eno2 also having found a
// neighbor, i.e. every interface networkd reported on has one.
const networkctlSampleAllFound = `{"Neighbors":[{"InterfaceIndex":4,"InterfaceName":"eno1","Neighbors":[{"ChassisID":"16:ae:d6:24:62:9f","PortID":"d0:11:e5:1c:6f:d8","SystemName":"switch01.example.com","VlanID":10}]},{"InterfaceIndex":5,"InterfaceName":"eno2","Neighbors":[{"ChassisID":"aa:bb:cc:dd:ee:ff","PortID":"eth3","SystemName":"switch02.example.com"}]}]}`

func TestQueryNetworkctlLLDPParsesResponse(t *testing.T) {
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		return []byte(networkctlSample), nil
	})

	neighbors, pending, ok := queryNetworkctlLLDP(context.Background(), logr.Discard())
	if !ok {
		t.Fatal("queryNetworkctlLLDP ok = false, want true")
	}
	if want := []string{"eno2"}; len(pending) != 1 || pending[0] != want[0] {
		t.Errorf("pending = %v, want %v (eno2 has an empty Neighbors list)", pending, want)
	}
	if len(neighbors) != 1 {
		t.Fatalf("neighbors = %+v, want exactly one entry (eno2 has an empty Neighbors list)", neighbors)
	}
	n, found := neighbors["eno1"]
	if !found {
		t.Fatalf("neighbors = %+v, want an eno1 entry", neighbors)
	}
	if got, want := *n.ChassisID, "16:ae:d6:24:62:9f"; got != want {
		t.Errorf("ChassisID = %q, want %q (first neighbor kept, not the second)", got, want)
	}
	if got, want := *n.SystemName, "switch01.example.com"; got != want {
		t.Errorf("SystemName = %q, want %q", got, want)
	}
	if got, want := n.VLANIDs, []uint32{10}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("VLANIDs = %v, want %v", got, want)
	}
}

func TestQueryNetworkctlLLDPOmitsZeroVlanID(t *testing.T) {
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		return []byte(`{"Neighbors":[{"InterfaceName":"eno1","Neighbors":[{"ChassisID":"aa"}]}]}`), nil
	})

	neighbors, _, ok := queryNetworkctlLLDP(context.Background(), logr.Discard())
	if !ok {
		t.Fatal("queryNetworkctlLLDP ok = false, want true")
	}
	if got := neighbors["eno1"].VLANIDs; got != nil {
		t.Errorf("VLANIDs = %v, want nil when networkd reports no VlanID", got)
	}
}

func TestQueryNetworkctlLLDPCommandError(t *testing.T) {
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		return nil, errors.New("exec: \"networkctl\": executable file not found in $PATH")
	})

	neighbors, _, ok := queryNetworkctlLLDP(context.Background(), logr.Discard())
	if ok {
		t.Errorf("queryNetworkctlLLDP ok = true, want false when the command fails")
	}
	if neighbors != nil {
		t.Errorf("neighbors = %v, want nil on command error", neighbors)
	}
}

func TestQueryNetworkctlLLDPInvalidJSON(t *testing.T) {
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		return []byte("not json"), nil
	})

	if _, _, ok := queryNetworkctlLLDP(context.Background(), logr.Discard()); ok {
		t.Error("queryNetworkctlLLDP ok = true, want false on unparseable output")
	}
}

func TestDiscoverLLDPViaNetworkdReturnsImmediatelyWhenFound(t *testing.T) {
	calls := 0
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		calls++
		return []byte(networkctlSampleAllFound), nil
	})

	neighbors, ok := discoverLLDPViaNetworkd(context.Background(), logr.Discard())
	if !ok {
		t.Fatal("discoverLLDPViaNetworkd ok = false, want true")
	}
	if len(neighbors) != 2 {
		t.Errorf("neighbors = %+v, want two entries", neighbors)
	}
	if calls != 1 {
		t.Errorf("networkctl called %d times, want 1 (every interface already had a neighbor on first query)", calls)
	}
}

func TestDiscoverLLDPViaNetworkdPollsUntilFound(t *testing.T) {
	orig := networkdPollInterval
	networkdPollInterval = time.Millisecond
	t.Cleanup(func() { networkdPollInterval = orig })
	withHasCarrier(t, func(string) bool { return true })

	calls := 0
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		calls++
		if calls < 3 {
			return []byte(`{"Neighbors":[{"InterfaceName":"eno1","Neighbors":[]},{"InterfaceName":"eno2","Neighbors":[]}]}`), nil
		}
		return []byte(networkctlSampleAllFound), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	neighbors, ok := discoverLLDPViaNetworkd(ctx, logr.Discard())
	if !ok {
		t.Fatal("discoverLLDPViaNetworkd ok = false, want true")
	}
	if len(neighbors) != 2 {
		t.Errorf("neighbors = %+v, want two entries once found", neighbors)
	}
	if calls != 3 {
		t.Errorf("networkctl called %d times, want 3 (polled until every interface had a neighbor)", calls)
	}
}

// TestDiscoverLLDPViaNetworkdPollsForSlowerInterfaces guards against the bug
// where finding a neighbor on one interface stopped discovery for the rest:
// eno1 already has one on the first query, but eno2's link came up moments
// later and doesn't get one until the third. Discovery must keep polling for
// eno2 rather than returning as soon as eno1 was satisfied.
func TestDiscoverLLDPViaNetworkdPollsForSlowerInterfaces(t *testing.T) {
	orig := networkdPollInterval
	networkdPollInterval = time.Millisecond
	t.Cleanup(func() { networkdPollInterval = orig })
	withHasCarrier(t, func(string) bool { return true })

	calls := 0
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		calls++
		if calls < 3 {
			return []byte(networkctlSample), nil // eno1 found, eno2 still empty
		}
		return []byte(networkctlSampleAllFound), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	neighbors, ok := discoverLLDPViaNetworkd(ctx, logr.Discard())
	if !ok {
		t.Fatal("discoverLLDPViaNetworkd ok = false, want true")
	}
	if _, found := neighbors["eno2"]; !found {
		t.Errorf("neighbors = %+v, want an eno2 entry once it caught up, not just eno1's", neighbors)
	}
	if len(neighbors) != 2 {
		t.Errorf("neighbors = %+v, want two entries", neighbors)
	}
	if calls != 3 {
		t.Errorf("networkctl called %d times, want 3 (kept polling for eno2 despite eno1 resolving first)", calls)
	}
}

// TestDiscoverLLDPViaNetworkdSkipsDownInterfaces guards a host with unused
// NICs (e.g. spare ports, an IPMI-only interface): networkd reports on them
// too but with nothing plugged in, they will never get an LLDP neighbor.
// Waiting the full timeout on them on every boot would be a needless startup
// delay, so discovery must return as soon as the only interfaces still
// missing a neighbor are down.
func TestDiscoverLLDPViaNetworkdSkipsDownInterfaces(t *testing.T) {
	orig := networkdPollInterval
	networkdPollInterval = time.Millisecond
	t.Cleanup(func() { networkdPollInterval = orig })
	withHasCarrier(t, func(string) bool { return false })

	calls := 0
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		calls++
		// eno1 found a neighbor, eno2 is plugged into nothing.
		return []byte(networkctlSample), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	neighbors, ok := discoverLLDPViaNetworkd(ctx, logr.Discard())
	if !ok {
		t.Fatal("discoverLLDPViaNetworkd ok = false, want true")
	}
	if len(neighbors) != 1 {
		t.Errorf("neighbors = %+v, want just eno1's entry", neighbors)
	}
	if calls != 1 {
		t.Errorf("networkctl called %d times, want 1 (eno2 has no carrier, not worth waiting on)", calls)
	}
}

// TestDiscoverLLDPViaNetworkdWaitsOnlyForUpInterfaces mixes one down
// interface (never gets a neighbor) with one up interface (never gets a
// neighbor either, e.g. connected to a switch that doesn't speak LLDP).
// Discovery must poll for the up one until the context times out, but must
// not treat the down one as a reason to keep going once the up one gives up.
func TestDiscoverLLDPViaNetworkdWaitsOnlyForUpInterfaces(t *testing.T) {
	orig := networkdPollInterval
	networkdPollInterval = time.Millisecond
	t.Cleanup(func() { networkdPollInterval = orig })
	withHasCarrier(t, func(name string) bool { return name == "eno1" })

	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		return []byte(`{"Neighbors":[{"InterfaceName":"eno1","Neighbors":[]},{"InterfaceName":"eno2","Neighbors":[]}]}`), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	neighbors, ok := discoverLLDPViaNetworkd(ctx, logr.Discard())
	if !ok {
		t.Error("discoverLLDPViaNetworkd ok = false, want true (networkd works, it just found no neighbors)")
	}
	if len(neighbors) != 0 {
		t.Errorf("neighbors = %+v, want empty - neither interface ever got one", neighbors)
	}
}

func TestDiscoverLLDPViaNetworkdFallsBackImmediatelyOnError(t *testing.T) {
	calls := 0
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		calls++
		return nil, errors.New("networkctl not found")
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, ok := discoverLLDPViaNetworkd(ctx, logr.Discard()); ok {
		t.Error("discoverLLDPViaNetworkd ok = true, want false when networkctl is unusable")
	}
	if calls != 1 {
		t.Errorf("networkctl called %d times, want 1 (no retry on a hard error)", calls)
	}
}

func TestDiscoverLLDPViaNetworkdReturnsEmptyAtTimeout(t *testing.T) {
	orig := networkdPollInterval
	networkdPollInterval = time.Millisecond
	t.Cleanup(func() { networkdPollInterval = orig })

	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		return []byte(`{"Neighbors":[]}`), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	neighbors, ok := discoverLLDPViaNetworkd(ctx, logr.Discard())
	if !ok {
		t.Error("discoverLLDPViaNetworkd ok = false, want true (networkd works, it just has no neighbors)")
	}
	if len(neighbors) != 0 {
		t.Errorf("neighbors = %+v, want empty once the timeout elapses", neighbors)
	}
}

func TestDiscoverLLDPReturnsNeighborsFromNetworkd(t *testing.T) {
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		return []byte(networkctlSample), nil
	})

	neighbors := DiscoverLLDP(context.Background(), logr.Discard(), time.Second)
	if len(neighbors) != 1 {
		t.Errorf("neighbors = %+v, want the one neighbor networkd reported", neighbors)
	}
}

// TestDiscoverLLDPSkipsGracefullyWithoutSystemd guards the HookOS case:
// its LinuxKit base has no systemd/networkd at all, so networkctl can't run,
// and in-band attribute discovery as a whole must not fail because of it.
func TestDiscoverLLDPSkipsGracefullyWithoutSystemd(t *testing.T) {
	withNetworkctlLLDP(t, func(context.Context) ([]byte, error) {
		return nil, errors.New("exec: \"networkctl\": executable file not found in $PATH")
	})

	if got := DiscoverLLDP(context.Background(), logr.Discard(), time.Second); got != nil {
		t.Errorf("DiscoverLLDP = %v, want nil when networkctl is unavailable", got)
	}
}
