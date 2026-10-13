package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path"
	"strconv"

	"github.com/go-logr/logr"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

// ArchiveName is the file name the Handler serves.
const ArchiveName = "bootstrap.cpio"

var errNetbootNotAllowed = errors.New("netboot is not allowed for this hardware: netboot.allowPXE is false in the Hardware record")

// BackendReader is the interface for getting hardware data from a backend.
type BackendReader interface {
	FilterHardware(ctx context.Context, opts data.HardwareFilter) (*tinkerbell.Hardware, error)
}

// Handler serves a Hardware's archive over HTTP, looked up by MAC address.
type Handler struct {
	Logger  logr.Logger
	Backend BackendReader
}

// HandlerFunc returns a handler for paths ending in <mac>/bootstrap.cpio.
func (h *Handler) HandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := h.Logger.WithValues("method", r.Method, "uri", r.RequestURI, "remoteAddr", r.RemoteAddr)

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if base := path.Base(r.URL.Path); base != ArchiveName {
			log.V(1).Info("unsupported bootstrap archive name", "name", base)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ha, err := macFromPath(r.URL.Path)
		if err != nil {
			log.Info("unable to parse mac address in the URL path", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		archive, err := h.archiveFor(r.Context(), ha)
		if err != nil {
			if hardwareNotFound(err) {
				log.Info("no hardware record for this machine", "mac", ha.String())
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if errors.Is(err, errNetbootNotAllowed) {
				log.Info("netboot not allowed for this hardware, netboot.allowPXE is false", "mac", ha.String())
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			log.Error(err, "unable to build the bootstrap archive", "mac", ha.String())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Generated in full before any bytes are written, so a failure is an
		// error response rather than a truncated archive the kernel would
		// silently drop.
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		if _, err := w.Write(archive); err != nil { //nolint:gosec // a CPIO archive served as application/octet-stream, not HTML
			log.Error(err, "unable to write the bootstrap archive", "mac", ha.String())
			return
		}
		log.V(1).Info("served bootstrap archive", "mac", ha.String(), "bytes", len(archive))
	}
}

func (h *Handler) archiveFor(ctx context.Context, mac net.HardwareAddr) ([]byte, error) {
	if h.Backend == nil {
		return nil, fmt.Errorf("no backend configured")
	}
	spec, err := h.Backend.FilterHardware(ctx, data.HardwareFilter{ByMACAddress: mac.String()})
	if err != nil {
		return nil, err
	}
	// Same gate as the iPXE script: the files often carry credentials.
	if !netbootAllowed(spec, mac) {
		return nil, errNetbootNotAllowed
	}

	files, err := Files(spec)
	if err != nil {
		return nil, err
	}

	return Archive(files, ModTime(spec))
}

// netbootAllowed reports the netboot.allowPXE of the interface with mac; unset is false.
func netbootAllowed(hw *tinkerbell.Hardware, mac net.HardwareAddr) bool {
	if hw == nil {
		return false
	}
	for _, i := range hw.Spec.Interfaces {
		if i.DHCP != nil && i.DHCP.MAC == mac.String() {
			return i.Netboot != nil && i.Netboot.AllowPXE != nil && *i.Netboot.AllowPXE
		}
	}
	return false
}

// hardwareNotFound reports whether err is a backend's not-found error.
func hardwareNotFound(err error) bool {
	var nf interface{ NotFound() bool }
	return errors.As(err, &nf) && nf.NotFound()
}

func macFromPath(urlPath string) (net.HardwareAddr, error) {
	mac := path.Base(path.Dir(urlPath))
	ha, err := net.ParseMAC(mac)
	if err != nil {
		return nil, fmt.Errorf("the second to last element in the URL path must be a valid mac address: %w", err)
	}

	return ha, nil
}
