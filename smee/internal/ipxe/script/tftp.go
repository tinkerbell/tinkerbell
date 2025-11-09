package script

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const pxeLinuxPrefix = "pxelinux.cfg/01-"

// pxeTemplate is the PXE extlinux.conf script template used to boot a machine via TFTP.
// IMPORTANT: U-Boot's PXE implementation requires RELATIVE paths for kernel/initrd.
// U-Boot will fetch these files using the same protocol (TFTP) that was used to fetch this config.
// The paths are relative to the TFTP server root directory.
var pxeTemplate = `
default deploy

label deploy
		kernel {{ .Kernel }}
		append console=tty1 console=ttyAMA0,115200 loglevel=7 cgroup_enable=cpuset cgroup_memory=1 cgroup_enable=memory {{- if ne .VLANID "" }} vlan_id={{ .VLANID }} {{- end }} facility={{ .Facility }} syslog_host={{ .SyslogHost }} grpc_authority={{ .TinkGRPCAuthority }} tinkerbell_tls={{ .TinkerbellTLS }} tinkerbell_insecure_tls={{ .TinkerbellInsecureTLS }} worker_id={{ .WorkerID }} hw_addr={{ .HWAddr }} modules=loop,squashfs,sd-mod,usb-storage intel_iommu=on iommu=pt {{- range .ExtraKernelParams}} {{.}} {{- end}}
		initrd {{ .Initrd }}
		ipappend 2

label local
    menu label Locally installed kernel
    append root=/dev/sda1
    localboot 1
`

// HandleTFTP implements the TFTP read function handler.
func (h Handler) HandleTFTP(filename string, rf io.ReaderFrom) error {
	// Extract MAC address from pxelinux.cfg/01-XX:XX:XX:XX:XX:XX format
	if !strings.HasPrefix(filename, pxeLinuxPrefix) {
		return fmt.Errorf("invalid pxelinux config filename: %s", filename)
	}

	macStr := strings.TrimPrefix(filename, pxeLinuxPrefix)
	log := h.Logger.WithValues("event", "pxelinux", "filename", filename, "macStr", macStr)

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
	if err != nil {
		log.Error(err, "backend lookup failed, using MAC address defaults", "mac", mac.String())
		return fmt.Errorf("failed to get machine info for MAC %s: %w", mac.String(), err)
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
