package script

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGenerateTemplate(t *testing.T) {
	tests := map[string]struct {
		h       Hook
		script  string
		want    string
		wantErr bool
	}{
		"no vlan": {
			h: Hook{
				Arch:              "x86_64",
				TinkGRPCAuthority: "1.2.3.4:42113",
				TinkerbellTLS:     false,
				WorkerID:          "3c:ec:ef:4c:4f:54",
				SyslogHost:        "1.2.3.4",
				DownloadURL:       "http://location:8080/to/kernel/and/initrd",
				Facility:          "onprem",
				ExtraKernelParams: []string{"tink_worker_image=quay.io/tinkerbell/tink-worker:v0.8.0", "tinkerbell=packet"},
				HWAddr:            "3c:ec:ef:4c:4f:54",
				Retries:           10,
				RetryDelay:        3,
				KernelName:        "vmlinuz-x86_64",
				InitrdName:        "initramfs-x86_64",
			},
			script: HookScript,
			want: `#!ipxe
# iPXE can only set the syslog server to an IP address, not a hostname (https://ipxe.org/cfg/syslog).
# If target is an IP, save it directly; if not, resolve it via nslookup directly into the syslog variable.
set check:ipv4 1.2.3.4 && set syslog 1.2.3.4 || nslookup syslog 1.2.3.4 || echo [WARN] Failed to resolve syslog host 1.2.3.4
clear check

echo Loading the Tinkerbell Hook iPXE script...

set arch x86_64
set download-url http://location:8080/to/kernel/and/initrd
set kernel vmlinuz-x86_64
set initrd initramfs-x86_64
set retries:int32 10
set retry_delay:int32 3

set idx:int32 0
:retry_kernel
kernel ${download-url}/${kernel} \
facility=onprem syslog_host=1.2.3.4 grpc_authority=1.2.3.4:42113 tinkerbell_tls=false tinkerbell_insecure_tls=false worker_id=3c:ec:ef:4c:4f:54 hw_addr=3c:ec:ef:4c:4f:54 \
modules=loop,squashfs,sd-mod,usb-storage intel_iommu=on iommu=pt initrd=${initrd} console=tty0 console=ttyS1,115200 tink_worker_image=quay.io/tinkerbell/tink-worker:v0.8.0 tinkerbell=packet && goto download_initrd || iseq ${idx} ${retries} && goto kernel-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_kernel

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
`,
		},
		"with vlan": {
			h: Hook{
				Arch:              "x86_64",
				TinkGRPCAuthority: "1.2.3.4:42113",
				TinkerbellTLS:     false,
				WorkerID:          "3c:ec:ef:4c:4f:54",
				SyslogHost:        "1.2.3.4",
				DownloadURL:       "http://location:8080/to/kernel/and/initrd",
				Facility:          "onprem",
				ExtraKernelParams: []string{"tink_worker_image=quay.io/tinkerbell/tink-worker:v0.8.0", "tinkerbell=packet"},
				HWAddr:            "3c:ec:ef:4c:4f:54",
				VLANID:            "16",
				Retries:           10,
				RetryDelay:        3,
				KernelName:        "vmlinuz-x86_64",
				InitrdName:        "initramfs-x86_64",
			},
			script: HookScript,
			want: `#!ipxe
# iPXE can only set the syslog server to an IP address, not a hostname (https://ipxe.org/cfg/syslog).
# If target is an IP, save it directly; if not, resolve it via nslookup directly into the syslog variable.
set check:ipv4 1.2.3.4 && set syslog 1.2.3.4 || nslookup syslog 1.2.3.4 || echo [WARN] Failed to resolve syslog host 1.2.3.4
clear check

echo Loading the Tinkerbell Hook iPXE script...

set arch x86_64
set download-url http://location:8080/to/kernel/and/initrd
set kernel vmlinuz-x86_64
set initrd initramfs-x86_64
set retries:int32 10
set retry_delay:int32 3

set idx:int32 0
:retry_kernel
kernel ${download-url}/${kernel} vlan_id=16 \
facility=onprem syslog_host=1.2.3.4 grpc_authority=1.2.3.4:42113 tinkerbell_tls=false tinkerbell_insecure_tls=false worker_id=3c:ec:ef:4c:4f:54 hw_addr=3c:ec:ef:4c:4f:54 \
modules=loop,squashfs,sd-mod,usb-storage intel_iommu=on iommu=pt initrd=${initrd} console=tty0 console=ttyS1,115200 tink_worker_image=quay.io/tinkerbell/tink-worker:v0.8.0 tinkerbell=packet && goto download_initrd || iseq ${idx} ${retries} && goto kernel-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_kernel

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
`,
		},
		"hostname syslog host": {
			h: Hook{
				Arch:              "x86_64",
				TinkGRPCAuthority: "1.2.3.4:42113",
				TinkerbellTLS:     false,
				WorkerID:          "3c:ec:ef:4c:4f:54",
				SyslogHost:        "syslog.example.com",
				DownloadURL:       "http://location:8080/to/kernel/and/initrd",
				Facility:          "onprem",
				ExtraKernelParams: []string{"tink_worker_image=quay.io/tinkerbell/tink-worker:v0.8.0", "tinkerbell=packet"},
				HWAddr:            "3c:ec:ef:4c:4f:54",
				Retries:           10,
				RetryDelay:        3,
				KernelName:        "vmlinuz-x86_64",
				InitrdName:        "initramfs-x86_64",
			},
			script: HookScript,
			want: `#!ipxe
# iPXE can only set the syslog server to an IP address, not a hostname (https://ipxe.org/cfg/syslog).
# If target is an IP, save it directly; if not, resolve it via nslookup directly into the syslog variable.
set check:ipv4 syslog.example.com && set syslog syslog.example.com || nslookup syslog syslog.example.com || echo [WARN] Failed to resolve syslog host syslog.example.com
clear check

echo Loading the Tinkerbell Hook iPXE script...

set arch x86_64
set download-url http://location:8080/to/kernel/and/initrd
set kernel vmlinuz-x86_64
set initrd initramfs-x86_64
set retries:int32 10
set retry_delay:int32 3

set idx:int32 0
:retry_kernel
kernel ${download-url}/${kernel} \
facility=onprem syslog_host=syslog.example.com grpc_authority=1.2.3.4:42113 tinkerbell_tls=false tinkerbell_insecure_tls=false worker_id=3c:ec:ef:4c:4f:54 hw_addr=3c:ec:ef:4c:4f:54 \
modules=loop,squashfs,sd-mod,usb-storage intel_iommu=on iommu=pt initrd=${initrd} console=tty0 console=ttyS1,115200 tink_worker_image=quay.io/tinkerbell/tink-worker:v0.8.0 tinkerbell=packet && goto download_initrd || iseq ${idx} ${retries} && goto kernel-error || inc idx && echo retry in ${retry_delay} seconds ; sleep ${retry_delay} ; goto retry_kernel

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
`,
		},
		"parse error": {
			h:       Hook{},
			script:  "bad {{ }",
			wantErr: true,
		},
		"execute error": {
			h:       Hook{},
			script:  "{{ .A }}",
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := GenerateTemplate(tt.h, tt.script)
			if (err != nil) != tt.wantErr {
				t.Errorf("Auto.autoDotIPXE() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Auto.autoDotIPXE() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHookSyslogIPXEConfigName(t *testing.T) {
	tests := map[string]struct {
		syslogHost string
		resolved   []string
		lookupErr  error
		wantLookup string
		want       string
	}{
		"ipv4":                       {syslogHost: "192.0.2.10", want: "syslog"},
		"ipv4 with port":             {syslogHost: "192.0.2.10:514", want: "syslog"},
		"ipv6":                       {syslogHost: "2001:db8::1", want: "syslog6"},
		"bracketed ipv6 port":        {syslogHost: "[2001:db8::1]:514", want: "syslog6"},
		"hostname resolves to ipv4":  {syslogHost: "syslog.example.com", resolved: []string{"192.0.2.10"}, wantLookup: "syslog.example.com", want: "syslog"},
		"hostname resolves to ipv6":  {syslogHost: "syslog.example.com", resolved: []string{"2001:db8::10"}, wantLookup: "syslog.example.com", want: "syslog6"},
		"hostname prefers any ipv6":  {syslogHost: "syslog.example.com", resolved: []string{"192.0.2.10", "2001:db8::10"}, wantLookup: "syslog.example.com", want: "syslog6"},
		"hostname with port":         {syslogHost: "syslog.example.com:514", resolved: []string{"2001:db8::10"}, wantLookup: "syslog.example.com", want: "syslog6"},
		"hostname lookup failure":    {syslogHost: "syslog.example.com", lookupErr: errors.New("lookup failed"), wantLookup: "syslog.example.com", want: "syslog"},
		"invalid resolved addresses": {syslogHost: "syslog.example.com", resolved: []string{"not-an-address"}, wantLookup: "syslog.example.com", want: "syslog"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			lookupCalled := false
			hook := Hook{
				SyslogHost: tt.syslogHost,
				lookupHost: func(host string) ([]string, error) {
					lookupCalled = true
					if host != tt.wantLookup {
						t.Fatalf("expected lookup for %q, got %q", tt.wantLookup, host)
					}
					return tt.resolved, tt.lookupErr
				},
			}
			if got := hook.SyslogIPXEConfigName(); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
			if lookupCalled != (tt.wantLookup != "") {
				t.Fatalf("lookup called = %v, want %v", lookupCalled, tt.wantLookup != "")
			}
		})
	}
}

func TestSyslogIPXEConfigNameInTemplates(t *testing.T) {
	for name, script := range map[string]string{
		"hook":   HookScript,
		"static": StaticScript,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := GenerateTemplate(Hook{
				SyslogHost: "syslog.example.com",
				lookupHost: func(string) ([]string, error) {
					return []string{"2001:db8::1"}, nil
				},
			}, script)
			if err != nil {
				t.Fatal(err)
			}
			want := "set check:ipv6 syslog.example.com && set syslog6 syslog.example.com || nslookup syslog6 syslog.example.com || echo [WARN] Failed to resolve syslog6 host syslog.example.com"
			if !strings.Contains(got, want) {
				t.Fatalf("expected IPv6 syslog resolution in script, got:\n%s", got)
			}
		})
	}
}
