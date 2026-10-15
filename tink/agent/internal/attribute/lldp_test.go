package attribute

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

// networkctlSample has the shape of real `networkctl lldp --json=short` output
// (https://github.com/tinkerbell/tinkerbell/pull/944#issuecomment-5424795070),
// with eno1's neighbors out of order and an extra eno2 entry that has none.
const networkctlSample = `{"Neighbors":[
{"InterfaceIndex":4,"InterfaceName":"eno1","Neighbors":[
  {"ChassisID":"bb:bb:bb:bb:bb:bb","PortID":"vnet0","SystemName":"vswitch01.example.com"},
  {"ChassisID":"16:ae:d6:24:62:9f","PortID":"d0:11:e5:1c:6f:d8","PortDescription":"en0","SystemName":"switch01.example.com","SystemDescription":"desc","EnabledCapabilities":12,"VlanID":10}]},
{"InterfaceIndex":5,"InterfaceName":"eno2","Neighbors":[]}]}`

// capturedLogs returns a logger that records what a user running at the default verbosity sees.
func capturedLogs() (logr.Logger, *strings.Builder) {
	var out strings.Builder
	l := funcr.New(func(prefix, args string) { out.WriteString(prefix + args + "\n") }, funcr.Options{})
	return l, &out
}

func TestLLDPQuery(t *testing.T) {
	tests := map[string]struct {
		out      string
		err      error
		want     map[string][]*data.LLDPNeighbor
		wantOK   bool
		wantLogs []string
	}{
		"parses and sorts neighbors": {
			out: networkctlSample,
			want: map[string][]*data.LLDPNeighbor{"eno1": {
				{ChassisID: toPtr("16:ae:d6:24:62:9f"), PortID: toPtr("d0:11:e5:1c:6f:d8"), PortDescription: toPtr("en0"), SystemName: toPtr("switch01.example.com")},
				{ChassisID: toPtr("bb:bb:bb:bb:bb:bb"), PortID: toPtr("vnet0"), SystemName: toPtr("vswitch01.example.com")},
			}},
			wantOK: true,
		},
		"unparseable output": {
			out:      "not json",
			wantLogs: []string{"could not parse the networkctl lldp output"},
		},
		"networkctl fails": {
			err:      errors.New("Failed to connect to bus"),
			wantLogs: []string{"networkctl lldp failed", "Failed to connect to bus"},
		},
		// Expected on hosts without systemd, such as HookOS, so it must be quiet.
		"networkctl not found": {
			err: &exec.Error{Name: "networkctl", Err: exec.ErrNotFound},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := lldp{networkctl: func(context.Context) ([]byte, error) { return []byte(tc.out), tc.err }}
			l, logs := capturedLogs()

			got, ok := d.query(context.Background(), l)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("neighbors mismatch (-want +got):\n%s", diff)
			}
			for _, want := range tc.wantLogs {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("logs = %q, want them to include %q", logs, want)
				}
			}
			if len(tc.wantLogs) == 0 && logs.Len() != 0 {
				t.Errorf("logs = %q, want nothing at the default verbosity", logs)
			}
		})
	}
}

func TestLLDPNeighbors(t *testing.T) {
	const (
		none     = `{"Neighbors":[]}`
		eno1     = `{"Neighbors":[{"InterfaceName":"eno1","Neighbors":[{"ChassisID":"aa"}]}]}`
		eno1And2 = `{"Neighbors":[{"InterfaceName":"eno1","Neighbors":[{"ChassisID":"aa"}]},{"InterfaceName":"eno2","Neighbors":[{"ChassisID":"bb"}]}]}`
	)
	type reply struct {
		out string
		err error
	}
	tests := map[string]struct {
		timeout time.Duration
		// replies[i] answers networkctl call i; the last one repeats.
		replies []reply
		// carrier is the content of /sys/class/net/<name>/carrier.
		carrier   map[string]string
		want      map[string][]string
		wantCalls int // -1 means any number of calls.
	}{
		"zero timeout disables discovery": {
			replies:   []reply{{out: eno1And2}},
			wantCalls: 0,
		},
		"networkctl unavailable": {
			timeout:   time.Second,
			replies:   []reply{{err: exec.ErrNotFound}},
			wantCalls: 1,
		},
		"every interface found on first query": {
			timeout:   time.Second,
			replies:   []reply{{out: eno1And2}},
			carrier:   map[string]string{"eno1": "1", "eno2": "1"},
			want:      map[string][]string{"eno1": {"aa"}, "eno2": {"bb"}},
			wantCalls: 1,
		},
		// networkctl omits interfaces without a neighbor, so discovery must
		// keep polling for eno2 even though the response never mentions it.
		"polls until every interface with a link has a neighbor": {
			timeout:   time.Second,
			replies:   []reply{{out: none}, {out: eno1}, {out: eno1And2}},
			carrier:   map[string]string{"eno1": "1", "eno2": "1"},
			want:      map[string][]string{"eno1": {"aa"}, "eno2": {"bb"}},
			wantCalls: 3,
		},
		"does not wait on interfaces without a link": {
			timeout:   time.Second,
			replies:   []reply{{out: eno1}},
			carrier:   map[string]string{"eno1": "1", "eno2": "0"},
			want:      map[string][]string{"eno1": {"aa"}},
			wantCalls: 1,
		},
		"returns what it has at the timeout": {
			timeout:   20 * time.Millisecond,
			replies:   []reply{{out: eno1}},
			carrier:   map[string]string{"eno1": "1", "eno2": "1"},
			want:      map[string][]string{"eno1": {"aa"}},
			wantCalls: -1,
		},
		"keeps earlier neighbors after a failed query": {
			timeout:   time.Second,
			replies:   []reply{{out: eno1}, {err: errors.New("transient")}},
			carrier:   map[string]string{"eno1": "1", "eno2": "1"},
			want:      map[string][]string{"eno1": {"aa"}},
			wantCalls: 2,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sysfs := t.TempDir()
			for iface, carrier := range tc.carrier {
				if err := os.MkdirAll(filepath.Join(sysfs, iface), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(sysfs, iface, "carrier"), []byte(carrier+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			d := lldp{
				networkctl: func(context.Context) ([]byte, error) {
					r := tc.replies[min(calls, len(tc.replies)-1)]
					calls++
					return []byte(r.out), r.err
				},
				sysClassNet:  sysfs,
				pollInterval: time.Millisecond,
			}

			got := d.neighbors(context.Background(), logr.Discard(), tc.timeout, []string{"eno1", "eno2"})

			var gotIDs map[string][]string
			if got != nil {
				gotIDs = map[string][]string{}
				for iface, ns := range got {
					for _, n := range ns {
						gotIDs[iface] = append(gotIDs[iface], *n.ChassisID)
					}
				}
			}
			if diff := cmp.Diff(tc.want, gotIDs); diff != "" {
				t.Errorf("neighbors mismatch (-want +got):\n%s", diff)
			}
			if tc.wantCalls >= 0 && calls != tc.wantCalls {
				t.Errorf("networkctl called %d times, want %d", calls, tc.wantCalls)
			}
		})
	}
}

func TestRunNetworkctlLLDPIncludesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'Failed to connect to bus' >&2; exit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "networkctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	_, err := runNetworkctlLLDP(context.Background())
	if err == nil {
		t.Fatal("runNetworkctlLLDP error = nil, want the command to fail")
	}
	for _, want := range []string{"Failed to connect to bus", "exit status 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to include %q", err, want)
		}
	}
}
