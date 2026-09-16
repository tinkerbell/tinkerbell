package script

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/smee/internal/hardware"
	"github.com/tinkerbell/tinkerbell/smee/internal/metric"
	"go.opentelemetry.io/otel/trace"
)

const x8664Arch = "x86_64"

// metric.Init registers collectors on the default registry and panics on a
// second registration, so tests that need the counters share one call.
var initMetrics = sync.OnceFunc(metric.Init)

func TestCustomScript(t *testing.T) {
	tests := map[string]struct {
		ipxeURL    string
		ipxeScript string
		want       string
		shouldErr  bool
	}{
		"got script":         {want: "#!ipxe\n\necho Loading custom Tinkerbell iPXE script...\n#!ipxe\nautoboot\n", ipxeScript: "#!ipxe\nautoboot"},
		"got url":            {want: "#!ipxe\n\necho Loading custom Tinkerbell iPXE script...\nchain --autofree https://boot.netboot.xyz\n", ipxeURL: "https://boot.netboot.xyz"},
		"invalid URL prefix": {want: "", ipxeURL: "invalid", shouldErr: true},
		"invalid URL":        {want: "", ipxeURL: "http://invalid.:123.com", shouldErr: true},
		"no script or url":   {want: "", shouldErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			h := &Handler{}
			u, err := url.Parse(tt.ipxeURL)
			if err != nil && !tt.shouldErr {
				t.Fatal(err)
			}

			d := hardware.Info{MACAddress: net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}, IPXEScript: tt.ipxeScript, IPXEScriptURL: u}
			got, err := h.customScript(d)
			if err != nil && !tt.shouldErr {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestDefaultScript(t *testing.T) {
	one := `#!ipxe

echo Loading the Tinkerbell Hook iPXE script...

set arch x86_64
set download-url http://127.1.1.1
set kernel vmlinuz-x86_64
set initrd initramfs-x86_64
set retries:int32 10
set retry_delay:int32 3

set idx:int32 0
:retry_kernel
kernel ${download-url}/${kernel} vlan_id=1234 \
facility=onprem syslog_host= grpc_authority=127.0.0.1:42113 tinkerbell_tls=false tinkerbell_insecure_tls=false worker_id=00:01:02:03:04:05 hw_addr=00:01:02:03:04:05 \
modules=loop,squashfs,sd-mod,usb-storage intel_iommu=on iommu=pt initrd=${initrd} console=tty0 console=ttyS1,115200 && goto download_initrd || iseq ${idx} ${retries} && goto kernel-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_kernel

:download_initrd
set idx:int32 0
:retry_initrd
initrd ${download-url}/${initrd} && goto boot || iseq ${idx} ${retries} && goto initrd-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_initrd

:boot
set idx:int32 0
:retry_boot
boot || iseq ${idx} ${retries} && goto boot-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_boot

:kernel-error
echo Failed to load kernel
imgfree
exit

:initrd-error
echo Failed to load initrd
imgfree
exit

:boot-error
echo Failed to boot
imgfree
exit
`

	two := `#!ipxe

echo Loading the Tinkerbell Hook iPXE script...

set arch x86_64
set download-url http://127.1.1.1
set kernel vmlinuz-x86_64
set initrd initramfs-x86_64
set retries:int32 10
set retry_delay:int32 3

set idx:int32 0
:retry_kernel
kernel ${download-url}/${kernel} vlan_id=1234 \
facility=onprem syslog_host= grpc_authority=127.0.0.1:42113 tinkerbell_tls=false tinkerbell_insecure_tls=false worker_id=worker1 hw_addr=00:01:02:03:04:05 \
modules=loop,squashfs,sd-mod,usb-storage intel_iommu=on iommu=pt initrd=${initrd} console=tty0 console=ttyS1,115200 && goto download_initrd || iseq ${idx} ${retries} && goto kernel-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_kernel

:download_initrd
set idx:int32 0
:retry_initrd
initrd ${download-url}/${initrd} && goto boot || iseq ${idx} ${retries} && goto initrd-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_initrd

:boot
set idx:int32 0
:retry_boot
boot || iseq ${idx} ${retries} && goto boot-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_boot

:kernel-error
echo Failed to load kernel
imgfree
exit

:initrd-error
echo Failed to load initrd
imgfree
exit

:boot-error
echo Failed to boot
imgfree
exit
`
	tests := map[string]struct {
		want string
		d    hardware.Info
	}{
		"success with defaults": {
			want: one,
			d:    hardware.Info{MACAddress: net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}, VLANID: "1234", Facility: "onprem", Arch: x8664Arch},
		},
		"success with set agent id": {
			want: two,
			d:    hardware.Info{MACAddress: net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}, AgentID: "worker1", VLANID: "1234", Facility: "onprem", Arch: x8664Arch},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			h := &Handler{
				OSIEURL:               "http://127.1.1.1",
				IPXEScriptRetries:     10,
				IPXEScriptRetryDelay:  3,
				TinkServerTLS:         false,
				TinkServerInsecureTLS: false,
				TinkServerGRPCAddr:    "127.0.0.1:42113",
				KernelName:            "vmlinuz",
				InitrdName:            "initramfs",
			}
			sp := trace.SpanFromContext(context.Background())
			got, err := h.defaultScript(sp, tt.d)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Log(got)
				t.Fatal(diff)
			}
		})
	}
}

func TestDefaultScriptCustomKernelInitrd(t *testing.T) {
	tests := map[string]struct {
		handler    Handler
		d          hardware.Info
		wantKernel string
		wantInitrd string
	}{
		"custom kernel and initrd names": {
			handler: Handler{
				OSIEURL:              "http://127.1.1.1",
				IPXEScriptRetries:    10,
				IPXEScriptRetryDelay: 3,
				TinkServerGRPCAddr:   "127.0.0.1:42113",
				KernelName:           "captain-kernel",
				InitrdName:           "captain-rootfs",
			},
			d:          hardware.Info{MACAddress: net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}, Facility: "onprem", Arch: x8664Arch},
			wantKernel: "captain-kernel-x86_64",
			wantInitrd: "captain-rootfs-x86_64",
		},
		"custom names with aarch64": {
			handler: Handler{
				OSIEURL:              "http://127.1.1.1",
				IPXEScriptRetries:    10,
				IPXEScriptRetryDelay: 3,
				TinkServerGRPCAddr:   "127.0.0.1:42113",
				KernelName:           "captain-kernel",
				InitrdName:           "captain-rootfs",
			},
			d:          hardware.Info{MACAddress: net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}, Facility: "onprem", Arch: "aarch64"},
			wantKernel: "captain-kernel-aarch64",
			wantInitrd: "captain-rootfs-aarch64",
		},
		"hw OSIE kernel/initrd overrides handler names": {
			handler: Handler{
				OSIEURL:              "http://127.1.1.1",
				IPXEScriptRetries:    10,
				IPXEScriptRetryDelay: 3,
				TinkServerGRPCAddr:   "127.0.0.1:42113",
				KernelName:           "captain-kernel",
				InitrdName:           "captain-rootfs",
			},
			d: hardware.Info{
				MACAddress: net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
				Facility:   "onprem",
				Arch:       x8664Arch,
				OSIE: hardware.OSIE{
					Kernel: "hw-specific-kernel",
					Initrd: "hw-specific-initrd",
				},
			},
			wantKernel: "hw-specific-kernel",
			wantInitrd: "hw-specific-initrd",
		},
		"hw OSIE kernel overrides but initrd uses handler": {
			handler: Handler{
				OSIEURL:              "http://127.1.1.1",
				IPXEScriptRetries:    10,
				IPXEScriptRetryDelay: 3,
				TinkServerGRPCAddr:   "127.0.0.1:42113",
				KernelName:           "captain-kernel",
				InitrdName:           "captain-rootfs",
			},
			d: hardware.Info{
				MACAddress: net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
				Facility:   "onprem",
				Arch:       x8664Arch,
				OSIE: hardware.OSIE{
					Kernel: "hw-specific-kernel",
				},
			},
			wantKernel: "hw-specific-kernel",
			wantInitrd: "captain-rootfs-x86_64",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			sp := trace.SpanFromContext(context.Background())
			got, err := tt.handler.defaultScript(sp, tt.d)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, "set kernel "+tt.wantKernel) {
				t.Errorf("expected kernel %q in script, got:\n%s", tt.wantKernel, got)
			}
			if !strings.Contains(got, "set initrd "+tt.wantInitrd) {
				t.Errorf("expected initrd %q in script, got:\n%s", tt.wantInitrd, got)
			}
		})
	}
}

func TestDefaultScriptKernelParams(t *testing.T) {
	h := Handler{
		OSIEURL:              "http://127.1.1.1",
		IPXEScriptRetries:    10,
		IPXEScriptRetryDelay: 3,
		TinkServerGRPCAddr:   "127.0.0.1:42113",
		KernelName:           "vmlinuz",
		InitrdName:           "initramfs",
		ExtraKernelParams:    []string{"global=1", "shared=global"},
	}
	hw := hardware.Info{
		MACAddress: net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
		Arch:       x8664Arch,
		OSIE: hardware.OSIE{
			KernelParams: []string{"perhw=1", "shared=perhw"},
		},
	}

	sp := trace.SpanFromContext(context.Background())
	got, err := h.defaultScript(sp, hw)
	if err != nil {
		t.Fatal(err)
	}

	for _, param := range []string{"global=1", "shared=global", "perhw=1", "shared=perhw"} {
		if !strings.Contains(got, param) {
			t.Errorf("expected kernel param %q in script, got:\n%s", param, got)
		}
	}
	// per-Hardware params must render after the global ones so machine-specific
	// values win on duplicate keys (kernel cmdline is last-wins).
	if strings.Index(got, "shared=perhw") < strings.Index(got, "shared=global") {
		t.Errorf("expected per-Hardware params to render after global params, got:\n%s", got)
	}
}

func TestStaticScript(t *testing.T) {
	want := `#!ipxe
# iPXE can only set the syslog server to an IP address, not a hostname (https://ipxe.org/cfg/syslog).
# If target is an IP, save it directly; if not, resolve it via nslookup directly into the syslog variable.
set check:ipv4 127.1.1.1 && set syslog 127.1.1.1 || nslookup syslog 127.1.1.1 || echo [WARN] Failed to resolve syslog host 127.1.1.1
clear check
echo Loading the static Tinkerbell iPXE script...

set arch ${buildarch}
# Tinkerbell only supports 64 bit architectures.
# The build architecture does not necessarily represent the architecture of the machine on which iPXE is running.
# https://ipxe.org/cfg/buildarch
iseq ${arch} i386 && set arch x86_64 ||
iseq ${arch} arm32 && set arch aarch64 ||
iseq ${arch} arm64 && set arch aarch64 ||

set kernel vmlinuz-${arch}
set initrd initramfs-${arch}
set download-url http://127.0.0.1
set retries:int32 0
set retry_delay:int32 0

set worker_id ${mac}
set grpc_authority 127.0.0.1:42113
set syslog_host 127.1.1.1
set tinkerbell_tls false

echo worker_id=${mac}
echo grpc_authority=127.0.0.1:42113
echo syslog_host=127.1.1.1
echo tinkerbell_tls=false

set idx:int32 0
:retry_kernel
kernel ${download-url}/${kernel} \
syslog_host=${syslog_host} grpc_authority=${grpc_authority} tinkerbell_tls=${tinkerbell_tls} worker_id=${worker_id} hw_addr=${mac} \
console=tty1 console=tty2 console=ttyAMA0,115200 console=ttyAMA1,115200 console=ttyS0,115200 console=ttyS1,115200 \
intel_iommu=on iommu=pt k=v k2=v2 initrd=${initrd} && goto download_initrd || iseq ${idx} ${retries} && goto kernel-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_kernel

:download_initrd
set idx:int32 0
:retry_initrd
initrd ${download-url}/${initrd} && goto boot || iseq ${idx} ${retries} && goto initrd-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_initrd

:boot
set idx:int32 0
:retry_boot
boot || iseq ${idx} ${retries} && goto boot-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_boot

:kernel-error
echo Failed to load kernel
imgfree
exit

:initrd-error
echo Failed to load initrd
imgfree
exit

:boot-error
echo Failed to boot
imgfree
exit
`
	initMetrics()
	h := &Handler{
		OSIEURL:            "http://127.0.0.1",
		ExtraKernelParams:  []string{"k=v", "k2=v2"},
		PublicSyslogFQDN:   "127.1.1.1",
		TinkServerTLS:      false,
		TinkServerGRPCAddr: "127.0.0.1:42113",
		StaticIPXEEnabled:  true,
		KernelName:         "vmlinuz",
		InitrdName:         "initramfs",
	}
	hf := h.HandlerFunc()
	writer := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auto.ipxe", nil)
	hf(writer, req)
	if writer.Code != 200 {
		t.Errorf("expected status code 200, got %d", writer.Code)
	}
	if diff := cmp.Diff(writer.Body.String(), want); diff != "" {
		t.Fatalf("expected custom script, got %s", diff)
	}
}

// mockBackend serves one canned Hardware (or an error) for every lookup.
type mockBackend struct {
	hw  *tinkerbell.Hardware
	err error
}

func (m *mockBackend) FilterHardware(_ context.Context, _ data.HardwareFilter) (*tinkerbell.Hardware, error) {
	return m.hw, m.err
}

// hardwareWithAllowPXE builds a minimal Hardware with one interface whose
// netboot.allowPXE is set as given.
func hardwareWithAllowPXE(mac string, allowPXE bool) *tinkerbell.Hardware {
	return &tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{
			Interfaces: []tinkerbell.Interface{
				{
					DHCP: &tinkerbell.DHCP{
						MAC: mac,
						IP: &tinkerbell.IP{
							Address: "192.168.1.100",
							Netmask: "255.255.255.0",
							Gateway: "192.168.1.1",
							Family:  4,
						},
					},
					Netboot: &tinkerbell.Netboot{AllowPXE: &allowPXE},
				},
			},
		},
	}
}

// The two ways an iPXE script request can be refused are different problems and
// must not look alike: no Hardware record at all is a 404, a Hardware record
// with netboot.allowPXE=false is a 403 that says so. In the L3 scenarios
// (external DHCP, static IPs, DHCP relay) Smee's DHCP handler never runs, so
// this response is the only place allowPXE=false is visible to the operator.
func TestHandlerFuncNetbootNotAllowed(t *testing.T) {
	// HandlerFunc counts requests; the collectors have to exist first.
	initMetrics()

	const mac = "aa:bb:cc:dd:ee:ff"

	tests := map[string]struct {
		backend    hardware.BackendReader
		wantStatus int
		wantInBody string
	}{
		"allowPXE false is 403 naming allowPXE": {
			backend:    &mockBackend{hw: hardwareWithAllowPXE(mac, false)},
			wantStatus: http.StatusForbidden,
			wantInBody: "allowPXE",
		},
		"no hardware record stays a 404": {
			backend:    &mockBackend{err: errors.New("no hardware")},
			wantStatus: http.StatusNotFound,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			h := &Handler{Logger: logr.Discard(), Backend: tt.backend}
			req := httptest.NewRequest(http.MethodGet, "/"+mac+"/auto.ipxe", nil)
			rr := httptest.NewRecorder()
			h.HandlerFunc()(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if tt.wantInBody != "" && !strings.Contains(rr.Body.String(), tt.wantInBody) {
				t.Errorf("body = %q, want it to mention %q", rr.Body.String(), tt.wantInBody)
			}
		})
	}
}
