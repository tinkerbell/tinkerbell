package tftp

import (
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/go-logr/logr"
)

// Handler represents a TFTP file handler.
type Handler interface {
	ServeTFTP(filename string, rf io.ReaderFrom) error
}

// HandlerFunc is an adapter to allow the use of ordinary functions as TFTP handlers.
type HandlerFunc func(filename string, rf io.ReaderFrom) error

// ServeTFTP calls f(filename, rf).
func (f HandlerFunc) ServeTFTP(filename string, rf io.ReaderFrom) error {
	return f(filename, rf)
}

// HandlerMapping is a map of routes to HandlerFuncs.
type HandlerMapping map[string]HandlerFunc

// patternHandler holds a compiled regex pattern and its associated handler.
type patternHandler struct {
	pattern *regexp.Regexp
	handler Handler
}

// ServeMux is a TFTP request multiplexer that matches filenames against
// registered regex patterns and routes them to the appropriate handler.
type ServeMux struct {
	defaultHandler Handler
	mu             sync.RWMutex
	patterns       []patternHandler
	log            logr.Logger
}

// NewServeMux allocates and returns a new ServeMux.
func NewServeMux() *ServeMux {
	return &ServeMux{}
}

// Handle registers the handler for the given regex pattern.
// If a pattern is malformed, Handle panics.
func (mux *ServeMux) Handle(pattern string, handler Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	regex, err := regexp.Compile(pattern)
	if err != nil {
		panic("tftp: invalid pattern " + pattern + ": " + err.Error())
	}

	mux.patterns = append(mux.patterns, patternHandler{
		pattern: regex,
		handler: handler,
	})
}

// HandleFunc registers the handler function for the given regex pattern.
func (mux *ServeMux) HandleFunc(pattern string, handler func(filename string, rf io.ReaderFrom) error) {
	mux.Handle(pattern, HandlerFunc(handler))
}

func (mux *ServeMux) SetDefaultHandler(handler Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()
	mux.defaultHandler = handler
}

func (mux *ServeMux) findHandler(filename string) (Handler, error) {
	mux.mu.RLock()
	defer mux.mu.RUnlock()

	// Find the first matching pattern
	for _, ph := range mux.patterns {
		if ph.pattern.MatchString(filename) {
			mux.log.V(2).Info("tftp request matched pattern",
				"filename", filename,
				"pattern", ph.pattern.String())
			return ph.handler, nil
		}
	}
	return nil, fmt.Errorf("no handler found for filename: %s", filename)
}

// ServeTFTP dispatches the request to the handler whose pattern most closely
// matches the request filename. If no handler is found, it returns an error.
func (mux *ServeMux) ServeTFTP(filename string, rf io.ReaderFrom) error {
	matchedHandler, err := mux.findHandler(filename)
	if err != nil {
		if mux.defaultHandler != nil {
			mux.log.V(2).Info("using default tftp handler for filename",
				"filename", filename)
			return mux.defaultHandler.ServeTFTP(filename, rf)
		}
		mux.log.Info("no tftp handler found for filename", "filename", filename)
		return ErrNotFound
	}

	if matchedHandler != nil {
		mux.log.V(2).Info("tftp request matched pattern",
			"filename", filename)
		return matchedHandler.ServeTFTP(filename, rf)
	}

	// No handler found
	mux.log.Info("no tftp handler found for filename", "filename", filename)
	return ErrNotFound
}

// NotFoundHandler returns a simple handler that replies to each request
// with a "404 file not found" error.
func NotFoundHandler() Handler {
	return HandlerFunc(func(_ string, _ io.ReaderFrom) error {
		return ErrNotFound
	})
}
