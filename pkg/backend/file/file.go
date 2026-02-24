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
	errFileFormat     = fmt.Errorf("invalid file format")
	errRecordNotFound = fmt.Errorf("record not found")
)

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

// ReadHardware is the implementation of the Backend interface.
// It reads hardware data from the in memory data (w.data) and searches for a match
// based on the provided ReadListOptions.
func (w *Watcher) ReadHardware(ctx context.Context, id, namespace string, opts data.ReadListOptions) (*tinkerbell.Hardware, error) {
	tracer := otel.Tracer(tracerName)
	_, span := tracer.Start(ctx, "backend.file.ReadHardware")
	defer span.End()

	w.dataMu.RLock()
	d := w.data
	w.dataMu.RUnlock()

	var hwList []tinkerbell.Hardware
	if err := yaml.Unmarshal(d, &hwList); err != nil {
		err := fmt.Errorf("%w: %w", err, errFileFormat)
		w.Log.Error(err, "failed to unmarshal file data")
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	for i := range hwList {
		hw := &hwList[i]
		if matchHardware(hw, id, opts) {
			span.SetStatus(codes.Ok, "")
			return hw, nil
		}
	}

	err := fmt.Errorf("%w: no matching hardware found", errRecordNotFound)
	span.SetStatus(codes.Error, err.Error())
	return nil, err
}

// matchHardware checks if a Hardware object matches the given search criteria.
func matchHardware(hw *tinkerbell.Hardware, id string, opts data.ReadListOptions) bool {
	if opts.ByName != "" && hw.Name == opts.ByName {
		return true
	}
	if opts.ByAgentID != "" && hw.Spec.AgentID == opts.ByAgentID {
		return true
	}
	if opts.Hardware.ByMACAddress != "" {
		for _, iface := range hw.Spec.Interfaces {
			if iface.DHCP != nil && strings.EqualFold(iface.DHCP.MAC, opts.Hardware.ByMACAddress) {
				return true
			}
		}
	}
	if opts.Hardware.ByIPAddress != "" {
		for _, iface := range hw.Spec.Interfaces {
			if iface.DHCP != nil && iface.DHCP.IP != nil && iface.DHCP.IP.Address == opts.Hardware.ByIPAddress {
				return true
			}
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
