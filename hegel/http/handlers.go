package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/packethost/pkg/log"
	"github.com/pkg/errors"
	"github.com/tinkerbell/hegel/build"
	"github.com/tinkerbell/hegel/datamodel"
	"github.com/tinkerbell/hegel/grpc"
	"github.com/tinkerbell/hegel/hardware"
	"github.com/tinkerbell/hegel/metrics"
)

// ec2Filters defines the query pattern and filters for the EC2 endpoint
// for queries that are to return another list of metadata items, the filter is a static list of the metadata items ("directory-listing filter")
// for /meta-data, the `spot` metadata item will only show up when the instance is a spot instance (denoted by if the `spot` field inside hardware is nonnull)
// NOTE: make sure when adding a new metadata item in a "subdirectory", to also add it to the directory-listing filter.
var ec2Filters = map[string]string{
	"":                                    `"meta-data", "user-data"`, // base path
	"/user-data":                          ".metadata.userdata",
	"/meta-data":                          `["instance-id", "hostname", "local-hostname", "iqn", "plan", "facility", "tags", "operating-system", "public-keys", "public-ipv4", "public-ipv6", "local-ipv4"] + (if .metadata.instance.spot != null then ["spot"] else [] end) | sort | .[]`,
	"/meta-data/instance-id":              ".metadata.instance.id",
	"/meta-data/hostname":                 ".metadata.instance.hostname",
	"/meta-data/local-hostname":           ".metadata.instance.hostname",
	"/meta-data/iqn":                      ".metadata.instance.iqn",
	"/meta-data/plan":                     ".metadata.instance.plan",
	"/meta-data/facility":                 ".metadata.instance.facility",
	"/meta-data/tags":                     ".metadata.instance.tags[]?",
	"/meta-data/operating-system":         `["slug", "distro", "version", "license_activation", "image_tag"] | sort | .[]`,
	"/meta-data/operating-system/slug":    ".metadata.instance.operating_system.slug",
	"/meta-data/operating-system/distro":  ".metadata.instance.operating_system.distro",
	"/meta-data/operating-system/version": ".metadata.instance.operating_system.version",
	"/meta-data/operating-system/license_activation":       `"state"`,
	"/meta-data/operating-system/license_activation/state": ".metadata.instance.operating_system.license_activation.state",
	"/meta-data/operating-system/image_tag":                ".metadata.instance.operating_system.image_tag",
	"/meta-data/public-keys":                               ".metadata.instance.ssh_keys[]?",
	"/meta-data/spot":                                      `"termination-time"`,
	"/meta-data/spot/termination-time":                     ".metadata.instance.spot.termination_time",
	"/meta-data/public-ipv4":                               ".metadata.instance.network.addresses[]? | select(.address_family == 4 and .public == true) | .address",
	"/meta-data/public-ipv6":                               ".metadata.instance.network.addresses[]? | select(.address_family == 6 and .public == true) | .address",
	"/meta-data/local-ipv4":                                ".metadata.instance.network.addresses[]? | select(.address_family == 4 and .public == false) | .address",
}

func VersionHandler(logger log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		payload := struct {
			// Use git_rev to match the health endpoint reporting.
			Revision string `json:"git_rev"`
		}{
			Revision: build.GetGitRevision(),
		}

		encoder := json.NewEncoder(w)

		if err := encoder.Encode(payload); err != nil {
			logger.Error(err, "marshalling version")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

// HealthChecker provide health checking behavior for services.
type HealthChecker interface {
	IsHealthy(context.Context) bool
}

// HealthCheckHandler provides an http handler that exposes health check information to consumers.
// The data is exposed as a json payload containing git_rev, uptim, goroutines and hardware_client_status.
func HealthCheckHandler(logger log.Logger, client HealthChecker, start time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIsHealthy := client.IsHealthy(r.Context())

		res := struct {
			GitRev                  string  `json:"git_rev"`
			Uptime                  float64 `json:"uptime"`
			Goroutines              int     `json:"goroutines"`
			HardwareClientAvailable bool    `json:"hardware_client_status"`
		}{
			GitRev:                  build.GetGitRevision(),
			Uptime:                  time.Since(start).Seconds(),
			Goroutines:              runtime.NumGoroutine(),
			HardwareClientAvailable: clientIsHealthy,
		}

		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)

		if err := encoder.Encode(&res); err != nil {
			logger.Error(err, "Failed to write for healthChecker")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !clientIsHealthy {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

// GetMetadataHandler provides an http handler that retrieves metadata using client filtering it
// using filter. filter should be a jq compatible processing string. Data is only filtered when
// using the TinkServer data model.
func GetMetadataHandler(logger log.Logger, client hardware.Client, filter string, model datamodel.DataModel) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		logger.Debug("retrieving metadata")
		userIP := getIPFromRequest(r)
		if userIP == "" {
			return
		}

		metrics.MetadataRequests.Inc()
		l := logger.With("userIP", userIP)
		l.Info("got ip from request")
		hw, err := client.ByIP(r.Context(), userIP)
		if err != nil {
			metrics.Errors.WithLabelValues("metadata", "lookup").Inc()
			l.With("error", err).Info("failed to get hardware by ip")
			w.WriteHeader(http.StatusNotFound)
			return
		}

		hardware, err := hw.Export()
		if err != nil {
			l.With("error", err).Info("failed to export hardware")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if model == datamodel.TinkServer {
			hardware, err = filterMetadata(hardware, filter)
			if err != nil {
				l.With("error", err).Info("failed to filter metadata")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		_, err = w.Write(hardware)
		if err != nil {
			l.With("error", err).Info("failed to write response")
		}
	})
}

func EC2MetadataHandler(logger log.Logger, client hardware.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		logger.Debug("calling EC2MetadataHandler")
		userIP := getIPFromRequest(r)
		if userIP == "" {
			return
		}

		metrics.MetadataRequests.Inc()
		l := logger.With("userIP", userIP)
		l.Info("got ip from request")
		hw, err := client.ByIP(r.Context(), userIP)
		if err != nil {
			metrics.Errors.WithLabelValues("metadata", "lookup").Inc()
			l.With("error", err).Info("failed to get hardware by ip")
			w.WriteHeader(http.StatusNotFound)
			return
		}

		filter, err := processEC2Query(r.URL.Path)
		if err != nil {
			l.With("error", err).Info("failed to process ec2 query")
			w.WriteHeader(http.StatusNotFound)
			_, err := w.Write([]byte("404 not found"))
			if err != nil {
				l.With("error", err).Info("failed to write response")
			}
			return
		}

		ehw, err := hw.Export()
		if err != nil {
			l.With("error", err).Info("failed to export hardware")
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte("404 not found"))
			if err != nil {
				l.With("error", err).Info("failed to write response")
			}
			return
		}
		resp, err := filterMetadata(ehw, filter)
		if err != nil {
			l.With("error", err).Info("failed to filter metadata")
		}

		w.WriteHeader(http.StatusOK)
		_, err = w.Write(resp)
		if err != nil {
			l.With("error", err).Info("failed to write response")
		}
	})
}

func filterMetadata(hw []byte, filter string) ([]byte, error) {
	var result bytes.Buffer
	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, err
	}
	input := make(map[string]interface{})
	err = json.Unmarshal(hw, &input)
	if err != nil {
		return nil, err
	}
	iter := query.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}

		if v == nil {
			continue
		}

		switch vv := v.(type) {
		case error:
			return nil, errors.Wrap(vv, "error while filtering with gojq")
		case string:
			result.WriteString(vv)
		default:
			marshalled, err := json.Marshal(vv)
			if err != nil {
				return nil, errors.Wrap(err, "error marshalling jq result")
			}
			result.Write(marshalled)
		}
		result.WriteRune('\n')
	}

	return bytes.TrimSuffix(result.Bytes(), []byte("\n")), nil
}

// processEC2Query returns either a specific filter (used to parse hardware data for the value of a specific field),
// or a comma-separated list of metadata items (to be printed).
func processEC2Query(url string) (string, error) {
	query := strings.TrimRight(strings.TrimPrefix(url, "/2009-04-04"), "/") // remove base pattern and trailing slash

	filter, ok := ec2Filters[query]
	if !ok {
		return "", errors.New("invalid metadata item")
	}

	return filter, nil
}

func getIPFromRequest(r *http.Request) string {
	addr := r.RemoteAddr
	if strings.ContainsRune(addr, ':') {
		addr, _, _ = net.SplitHostPort(addr)
	}
	return addr
}

func writeJSON(logger log.Logger, w http.ResponseWriter, status int, data interface{}) error {
	var body []byte

	body, err := json.Marshal(data)
	if err != nil {
		if status < 400 {
			return jsonError(logger, w, http.StatusInternalServerError, err, "marshalling response")
		}

		status = 500
		logger.Error(err, "failed to marshal error")
		body = []byte(`{"error", {"comment": "Failed to marshal error"}}`)
	}

	w.WriteHeader(status)
	_, err = w.Write(body)
	return err
}

func jsonError(logger log.Logger, w http.ResponseWriter, status int, err error, msg string) error {
	logger.Error(err, msg)
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"error":   err.Error(),
			"comment": msg,
		},
	}
	return writeJSON(logger, w, status, resp)
}

// todo(chrisdoherty4) Re-write this. It violates several laws and is dangerously fragile.
// Also, does this work for `/subscriptions`? The writeJSON call injects a function that isn't
// handled properly by writeJSON.
func SubscriptionsHandler(server *grpc.Server, logger log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getid := strings.TrimPrefix(r.URL.Path, "/subscriptions/")
		server.SubLock().RLock()
		defer server.SubLock().RUnlock()
		var err error
		if getid == "" {
			err = writeJSON(logger, w, http.StatusOK, server.Subscriptions)
		} else if sub, ok := server.Subscriptions()[getid]; ok {
			err = writeJSON(logger, w, http.StatusOK, sub)
		} else {
			err = jsonError(logger, w, http.StatusNotFound, errors.Errorf("%s not found", getid), "item not found")
		}
		if err != nil {
			logger.Error(err)
		}
	})
}
