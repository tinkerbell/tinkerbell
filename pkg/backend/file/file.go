// Package file watches a file for changes and updates the in memory hardware data.
package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"sigs.k8s.io/yaml"
)

const tracerName = "github.com/tinkerbell/tinkerbell"

// Errors used by the file watcher.
var (
	// errFileFormat is returned when the file is not in the correct format, e.g. not valid YAML.
	errFileFormat = fmt.Errorf("invalid file format")
)

type hardwareNotFoundError struct{}

func (hardwareNotFoundError) NotFound() bool { return true }

func (hardwareNotFoundError) Error() string {
	return "no matching hardware found"
}

type foundMultipleHardwareError struct {
	count int
}

func (f foundMultipleHardwareError) MultipleFound() bool { return true }

func (f foundMultipleHardwareError) Error() string {
	return fmt.Sprintf("found %d hardware objects matching filter, expected 1", f.count)
}

// Watcher represents the backend for watching a file for changes and updating the in memory DHCP data.
type Watcher struct {
	fileMu sync.RWMutex // protects FilePath for reads

	// FilePath is the path to the file to watch.
	FilePath string

	// Log is the logger to be used in the File backend.
	Log     logr.Logger
	dataMu  sync.RWMutex // protects data
	data    []byte       // data from file
	watcher *fsnotify.Watcher
}

// NewWatcher creates a new file watcher.
func NewWatcher(l logr.Logger, f string) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(f); err != nil {
		return nil, err
	}

	w := &Watcher{
		FilePath: f,
		watcher:  watcher,
		Log:      l,
	}

	w.fileMu.RLock()
	w.data, err = os.ReadFile(filepath.Clean(f))
	w.fileMu.RUnlock()
	if err != nil {
		return nil, err
	}

	return w, nil
}

// ReadHardware looks up a Hardware object by name from the in-memory data.
func (w *Watcher) ReadHardware(ctx context.Context, name, namespace string) (*tinkerbell.Hardware, error) {
	tracer := otel.Tracer(tracerName)
	_, span := tracer.Start(ctx, "backend.file.ReadHardware")
	defer span.End()

	w.dataMu.RLock()
	d := w.data
	w.dataMu.RUnlock()

	var hwList []tinkerbell.Hardware
	if err := yaml.Unmarshal(d, &hwList); err != nil {
		err := fmt.Errorf("%w: %v", errFileFormat, err)
		w.Log.Error(err, "failed to unmarshal file data")
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	for i := range hwList {
		hw := &hwList[i]
		if namespace != "" && hw.Namespace != namespace {
			continue
		}
		if hw.Name == name {
			span.SetStatus(codes.Ok, "")
			return hw, nil
		}
	}

	err := hardwareNotFoundError{}
	span.SetStatus(codes.Error, err.Error())
	return nil, err
}

// FilterHardware looks up a single Hardware object from the in-memory data using selector-based filtering.
// Exactly one result is expected; zero results returns a not-found error and multiple results returns a multiple-found error.
func (w *Watcher) FilterHardware(ctx context.Context, opts data.HardwareFilter) (*tinkerbell.Hardware, error) {
	tracer := otel.Tracer(tracerName)
	_, span := tracer.Start(ctx, "backend.file.FilterHardware")
	defer span.End()

	w.dataMu.RLock()
	d := w.data
	w.dataMu.RUnlock()

	var hwList []tinkerbell.Hardware
	if err := yaml.Unmarshal(d, &hwList); err != nil {
		err := fmt.Errorf("%w: %v", errFileFormat, err)
		w.Log.Error(err, "failed to unmarshal file data")
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var matches []*tinkerbell.Hardware
	for i := range hwList {
		hw := &hwList[i]
		if matchHardware(hw, opts) {
			matches = append(matches, hw)
		}
	}

	switch len(matches) {
	case 0:
		err := hardwareNotFoundError{}
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	case 1:
		span.SetStatus(codes.Ok, "")
		return matches[0], nil
	default:
		err := foundMultipleHardwareError{count: len(matches)}
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
}

// UpdateHardware is not supported by the file backend as it is read-only.
func (w *Watcher) UpdateHardware(_ context.Context, _ *tinkerbell.Hardware, _ data.UpdateOptions) error {
	return fmt.Errorf("file backend does not support hardware updates")
}

// matchHardware checks if a Hardware object matches all the given filter selectors (AND logic).
func matchHardware(hw *tinkerbell.Hardware, opts data.HardwareFilter) bool {
	if opts.InNamespace != "" && hw.Namespace != opts.InNamespace {
		return false
	}
	if opts.ByName != "" && hw.Name != opts.ByName {
		return false
	}
	if opts.ByAgentID != "" && hw.Spec.AgentID != opts.ByAgentID {
		return false
	}
	if opts.ByMACAddress != "" && !hardwareHasMAC(hw, opts.ByMACAddress) {
		return false
	}
	if opts.ByIPAddress != "" && !hardwareHasIP(hw, opts.ByIPAddress) {
		return false
	}
	if opts.ByInstanceID != "" && (hw.Spec.Metadata == nil || hw.Spec.Metadata.Instance == nil || hw.Spec.Metadata.Instance.ID != opts.ByInstanceID) {
		return false
	}
	// At least one selector must be set for a match.
	return opts.ByName != "" || opts.ByAgentID != "" || opts.ByMACAddress != "" || opts.ByIPAddress != "" || opts.ByInstanceID != ""
}

func hardwareHasMAC(hw *tinkerbell.Hardware, mac string) bool {
	for _, iface := range hw.Spec.Interfaces {
		if iface.DHCP != nil && strings.EqualFold(iface.DHCP.MAC, mac) {
			return true
		}
	}
	return false
}

func hardwareHasIP(hw *tinkerbell.Hardware, ip string) bool {
	for _, iface := range hw.Spec.Interfaces {
		if iface.DHCP != nil && iface.DHCP.IP != nil && iface.DHCP.IP.Address == ip {
			return true
		}
	}
	return false
}

// Start starts watching a file for changes and updates the in memory data (w.data) on changes.
// Start is a blocking method. Use a context cancellation to exit.
func (w *Watcher) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.Log.Info("stopping watcher")
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				continue
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				w.Log.Info("file changed, updating cache")
				w.fileMu.RLock()
				d, err := os.ReadFile(w.FilePath)
				w.fileMu.RUnlock()
				if err != nil {
					w.Log.Error(err, "failed to read file", "file", w.FilePath)
					break
				}
				w.dataMu.Lock()
				w.data = d
				w.dataMu.Unlock()
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				continue
			}
			w.Log.Info("error watching file", "err", err)
		}
	}
}
