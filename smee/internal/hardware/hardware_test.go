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
// has both DHCP and Netboot set, optionally with PXELINUX and RPI fields and
// an OSIE.KernelParams list.
func validHardware(mac, ip string, allowPXE bool, pxelinuxConfig, rpiSerial string, kernelParams []string) *tinkerbell.Hardware {
	return &tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{
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
						OSIE:     &tinkerbell.OSIE{KernelParams: kernelParams},
						PXELINUX: &tinkerbell.PXELINUX{Config: pxelinuxConfig},
						RPI: &tinkerbell.RPI{
							SerialNum:    rpiSerial,
							FirmwarePath: "rpi4",
						},
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
		backend          BackendReader
		wantErr          bool
		wantConfig       string
		wantRPiSerial    string
		wantKernelParams []string
	}{
		"nil backend": {
			backend: nil,
			wantErr: true,
		},
		"backend errors": {
			backend: &mockBackend{err: errBackend},
			wantErr: true,
		},
		"success populates PXELINUX, RPI and OSIE.KernelParams": {
			backend: &mockBackend{
				hw: validHardware(mac.String(), "192.168.1.100", true, "default linux kernel append", "abc123", []string{"console=tty1", "rw"}),
			},
			wantConfig:       "default linux kernel append",
			wantRPiSerial:    "abc123",
			wantKernelParams: []string{"console=tty1", "rw"},
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
			if diff := cmp.Diff(info.PXELINUX.Config, tt.wantConfig); diff != "" {
				t.Fatalf("PXELINUX.Config mismatch: %s", diff)
			}
			if diff := cmp.Diff(info.RPI.SerialNum, tt.wantRPiSerial); diff != "" {
				t.Fatalf("RPI.SerialNum mismatch: %s", diff)
			}
			if diff := cmp.Diff(info.OSIE.KernelParams, tt.wantKernelParams); diff != "" {
				t.Fatalf("OSIE.KernelParams mismatch: %s", diff)
			}
		})
	}
}

func TestGetByIP(t *testing.T) {
	errBackend := errors.New("boom")
	ip := net.ParseIP("192.168.1.100")

	tests := map[string]struct {
		backend          BackendReader
		wantErr          bool
		wantConfig       string
		wantRPiSerial    string
		wantKernelParams []string
	}{
		"nil backend": {
			backend: nil,
			wantErr: true,
		},
		"backend errors": {
			backend: &mockBackend{err: errBackend},
			wantErr: true,
		},
		"success populates PXELINUX, RPI and OSIE.KernelParams": {
			backend: &mockBackend{
				hw: validHardware("01:02:03:04:05:06", ip.String(), true, "cmdline overrides", "serial42", []string{"quiet"}),
			},
			wantConfig:       "cmdline overrides",
			wantRPiSerial:    "serial42",
			wantKernelParams: []string{"quiet"},
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
			if diff := cmp.Diff(info.PXELINUX.Config, tt.wantConfig); diff != "" {
				t.Fatalf("PXELINUX.Config mismatch: %s", diff)
			}
			if diff := cmp.Diff(info.RPI.SerialNum, tt.wantRPiSerial); diff != "" {
				t.Fatalf("RPI.SerialNum mismatch: %s", diff)
			}
			if diff := cmp.Diff(info.OSIE.KernelParams, tt.wantKernelParams); diff != "" {
				t.Fatalf("OSIE.KernelParams mismatch: %s", diff)
			}
		})
	}
}

func TestBackendResolver(t *testing.T) {
	mac := net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	ip := net.ParseIP("192.168.1.100")
	be := &mockBackend{
		hw: validHardware(mac.String(), ip.String(), true, "tmpl", "serial1", nil),
	}
	r := BackendResolver{Backend: be}

	t.Run("ByMAC delegates to GetByMac", func(t *testing.T) {
		info, err := r.ByMAC(context.Background(), mac)
		if err != nil {
			t.Fatal(err)
		}
		if info.PXELINUX.Config != "tmpl" {
			t.Fatalf("unexpected config: %q", info.PXELINUX.Config)
		}
	})

	t.Run("ByIP delegates to GetByIP", func(t *testing.T) {
		info, err := r.ByIP(context.Background(), ip)
		if err != nil {
			t.Fatal(err)
		}
		if info.RPI.SerialNum != "serial1" {
			t.Fatalf("unexpected serial: %q", info.RPI.SerialNum)
		}
	})
}
