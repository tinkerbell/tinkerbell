package hardware

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

type mockBackend struct {
	hw  *tinkerbell.Hardware
	err error
}

func (m *mockBackend) FilterHardware(_ context.Context, _ data.HardwareFilter) (*tinkerbell.Hardware, error) {
	return m.hw, m.err
}

// validHardware returns a minimal tinkerbell.Hardware with one interface that
// has both DHCP and Netboot set.
func validHardware(mac, ip string, allowPXE bool, facility string) *tinkerbell.Hardware {
	return &tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{
			Metadata: &tinkerbell.HardwareMetadata{
				Facility: &tinkerbell.MetadataFacility{FacilityCode: facility},
			},
			Interfaces: []tinkerbell.Interface{
				{
					DHCP: &tinkerbell.DHCP{
						MAC: mac,
						IP: &tinkerbell.IP{
							Address: ip,
							Netmask: "255.255.255.0",
							Gateway: "192.168.1.1",
							Family:  4,
						},
					},
					Netboot: &tinkerbell.Netboot{
						AllowPXE: &allowPXE,
					},
				},
			},
		},
	}
}

func TestGetByMac(t *testing.T) {
	errBackend := errors.New("boom")
	mac := net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}

	tests := map[string]struct {
		backend      BackendReader
		wantErr      bool
		wantFacility string
	}{
		"nil backend": {
			backend: nil,
			wantErr: true,
		},
		"backend errors": {
			backend: &mockBackend{err: errBackend},
			wantErr: true,
		},
		"success populates Info": {
			backend: &mockBackend{
				hw: validHardware(mac.String(), "192.168.1.100", true, "onprem"),
			},
			wantFacility: "onprem",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			info, err := GetByMac(context.Background(), mac, tt.backend)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil; info=%+v", info)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(info.Facility, tt.wantFacility); diff != "" {
				t.Fatalf("Facility mismatch: %s", diff)
			}
		})
	}
}

func TestGetByIP(t *testing.T) {
	errBackend := errors.New("boom")
	ip := net.ParseIP("192.168.1.100")

	tests := map[string]struct {
		backend      BackendReader
		wantErr      bool
		wantFacility string
	}{
		"nil backend": {
			backend: nil,
			wantErr: true,
		},
		"backend errors": {
			backend: &mockBackend{err: errBackend},
			wantErr: true,
		},
		"success populates Info": {
			backend: &mockBackend{
				hw: validHardware("01:02:03:04:05:06", ip.String(), true, "remote"),
			},
			wantFacility: "remote",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			info, err := GetByIP(context.Background(), ip, tt.backend)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil; info=%+v", info)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(info.Facility, tt.wantFacility); diff != "" {
				t.Fatalf("Facility mismatch: %s", diff)
			}
		})
	}
}

func TestBackendResolver(t *testing.T) {
	mac := net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	ip := net.ParseIP("192.168.1.100")
	be := &mockBackend{
		hw: validHardware(mac.String(), ip.String(), true, "lab"),
	}
	r := BackendResolver{Backend: be}

	t.Run("ByMAC delegates to GetByMac", func(t *testing.T) {
		info, err := r.ByMAC(context.Background(), mac)
		if err != nil {
			t.Fatal(err)
		}
		if info.Facility != "lab" {
			t.Fatalf("unexpected facility: %q", info.Facility)
		}
	})

	t.Run("ByIP delegates to GetByIP", func(t *testing.T) {
		info, err := r.ByIP(context.Background(), ip)
		if err != nil {
			t.Fatal(err)
		}
		if info.Facility != "lab" {
			t.Fatalf("unexpected facility: %q", info.Facility)
		}
	})
}
