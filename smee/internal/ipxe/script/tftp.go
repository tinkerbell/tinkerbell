package script

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// pxeLinuxPattern matches pxelinux.cfg/<hwtype>-<mac> where hwtype is 2-digit hex (e.g., "01" for Ethernet)
// Per RFC 2132 and syslinux specification: hardware type is specified as 2-digit hexadecimal
var pxeLinuxPattern = regexp.MustCompile(`^pxelinux\.cfg/([0-9a-fA-F]{2})-(.+)$`)

// pxeTemplate is the PXE extlinux.conf script template used to boot a machine via TFTP.
// IMPORTANT: U-Boot's PXE implementation requires RELATIVE paths for kernel/initrd.
// U-Boot will fetch these files using the same protocol (TFTP) that was used to fetch this config.
// The paths are relative to the TFTP server root directory.
var pxeTemplate = `
default deploy

label deploy
		kernel {{ if .Kernel }}{{ .Kernel }}{{ else }}vmlinuz-{{ .Arch }}{{ end }}
		append console=tty1 console=ttyAMA0,115200 loglevel=7 cgroup_enable=cpuset cgroup_memory=1 cgroup_enable=memory \
		net.ifnames=0 {{- if ne .VLANID "" }} vlan_id={{ .VLANID }} {{- end }} facility={{ .Facility }} syslog_host={{ .SyslogHost }} grpc_authority={{ .TinkGRPCAuthority }} \
		tinkerbell_tls={{ .TinkerbellTLS }} tinkerbell_insecure_tls={{ .TinkerbellInsecureTLS }} worker_id={{ .WorkerID }} hw_addr={{ .HWAddr }} \
		modules=loop,squashfs,sd-mod,usb-storage intel_iommu=on iommu=pt initrd=initramfs-{{ .Arch }} {{- range .ExtraKernelParams}} {{.}} {{- end}}
		initrd {{ if .Initrd }}{{ .Initrd }}{{ else }}initramfs-{{ .Arch }}{{ end }}
		ipappend 2

label local
    menu label Locally installed kernel
    append root=/dev/sda1
    localboot 1
`

// HandleTFTP implements the TFTP read function handler.
func (h Handler) HandleTFTP(filename string, rf io.ReaderFrom) error {
	// Extract hardware type and MAC address from pxelinux.cfg/<hwtype>-<mac> format
	// Per RFC 2132 and syslinux: hardware type is 2-digit hex (e.g., "01" for Ethernet)
	matches := pxeLinuxPattern.FindStringSubmatch(filename)
	if matches == nil || len(matches) != 3 {
		return fmt.Errorf("invalid pxelinux config filename format: %s (expected: pxelinux.cfg/<hwtype>-<mac>)", filename)
	}

	hwTypeStr := matches[1]
	macStr := matches[2]
	log := h.Logger.WithValues("event", "pxelinux", "filename", filename, "hwType", hwTypeStr, "macStr", macStr)

	// Parse MAC address
	mac, err := net.ParseMAC(macStr)
	if err != nil {
		log.Info("invalid MAC address for pxelinux.cfg request", "error", err)
		return fmt.Errorf("invalid MAC address for pxelinux.cfg request: %w", err)
	}

	// Create tracing context
	tracer := otel.Tracer("TFTP-PXELinux")
	ctx, span := tracer.Start(context.Background(), "TFTP pxelinux.cfg generation",
		trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	// Get machine data from backend
	hw, err := getByMac(ctx, mac, h.Backend)
	if err != nil && !h.StaticIPXEEnabled {
		log.Error(err, "backend lookup failed, using MAC address defaults", "mac", mac.String())
		return fmt.Errorf("failed to get machine info for MAC %s: %w", mac.String(), err)
	}

	if err != nil && h.StaticIPXEEnabled {
		arch := "x86_64"
		if dhcp.IsRaspberryPI(mac) {
			arch = "aarch64"
		}
		hw = info{
			Arch:         arch,
			MACAddress:   mac,
			AllowNetboot: true,
		}
	}

	if !hw.AllowNetboot {
		e := errors.New("netboot not allowed for this machine")
		span.SetStatus(codes.Error, e.Error())
		log.Error(e, "mac", mac.String())
		return e
	}

	// Build Hook struct from hardware info
	auto := h.buildHook(span, hw)

	// Generate the iPXE script content
	content, err := GenerateTemplate(auto, pxeTemplate)
	if err != nil {
		e := fmt.Errorf("failed to generate pxelinux config: %w", err)
		log.Error(e, "failed to generate pxelinux config")
		span.SetStatus(codes.Error, e.Error())
		return e
	}

	log.Info("serving generated pxelinux config", "size", len(content))

	// Serve the content
	return serveContent(content, rf, log, span, filename)
}

func serveContent(content string, rf io.ReaderFrom, log logr.Logger, span trace.Span, filename string) error {
	if transfer, ok := rf.(interface{ SetSize(int64) }); ok {
		transfer.SetSize(int64(len(content)))
	}

	reader := strings.NewReader(content)

	bytesRead, err := rf.ReadFrom(reader)
	if err != nil {
		log.Error(err, "file serve failed", "bytesRead", bytesRead, "contentSize", len(content))
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	log.Info("file served", "bytesSent", bytesRead, "contentSize", len(content))
	span.SetStatus(codes.Ok, filename)
	return nil
}
