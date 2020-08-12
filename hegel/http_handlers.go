package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/packethost/hegel/metrics"
)

var (
	// ec2Filters defines the query pattern and filters for the EC2 endpoint
	// for queries that are to return another list of metadata items, the filter is a static list of the metadata items ("directory-listing filter")
	// for /meta-data, the `spot` metadata item will only show up when the instance is a spot instance (denoted by if the `spot` field inside hardware is nonnull)
	// NOTE: make sure when adding a new metadata item in a "subdirectory", to also add it to the directory-listing filter
	ec2Filters = map[string]string{
		"":                                    `"meta-data", "user-data"`, // base path
		"/user-data":                          ".metadata.userdata",
		"/meta-data":                          `["instance-id", "hostname", "iqn", "plan", "facility", "tags", "operating-system", "public-keys", "public-ipv4", "public-ipv6", "local-ipv4"] + (if .metadata.instance.spot != null then ["spot"] else [] end) | sort | .[]`,
		"/meta-data/instance-id":              ".metadata.instance.id",
		"/meta-data/hostname":                 ".metadata.instance.hostname",
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
)

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write(gitRevJSON)
	if err != nil {
		logger.Error(err, " Failed to write gitRevJSON")
	}
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	isCacherAvailableMu.RLock()
	isCacherAvailableTemp := isCacherAvailable
	isCacherAvailableMu.RUnlock()

	res := struct {
		GitRev          string  `json:"git_rev"`
		Uptime          float64 `json:"uptime"`
		Goroutines      int     `json:"goroutines"`
		CacherAvailable bool    `json:"cacher_status"`
	}{
		GitRev:          gitRev,
		Uptime:          time.Since(StartTime).Seconds(),
		Goroutines:      runtime.NumGoroutine(),
		CacherAvailable: isCacherAvailableTemp,
	}
	b, err := json.Marshal(&res)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	if !isCacherAvailableTemp {
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(b)
	if err != nil {
		logger.Error(err, " Failed to write for healthChecker")
	}
}

func getMetadata(filter string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		logger.Debug("calling getMetadata ")
		userIP := getIPFromRequest(r)
		if userIP == "" {
			return
		}

		metrics.MetadataRequests.Inc()
		l := logger.With("userIP", userIP)
		l.Info("got ip from request")
		hw, err := getByIP(context.Background(), hegelServer, userIP) // returns hardware data as []byte
		if err != nil {
			metrics.Errors.WithLabelValues("metadata", "lookup").Inc()
			l.With("error", err).Info("failed to get hardware by ip")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var resp []byte
		dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
		switch dataModelVersion {
		case "":
			resp, err = exportHardware(hw) // in cacher mode, the "filter" is the exportedHardwareCacher type
			if err != nil {
				l.With("error", err).Info("failed to export hardware")
			}
		case "1":
			resp, err = filterMetadata(hw, filter)
			if err != nil {
				l.With("error", err).Info("failed to filter metadata")
			}
		default:
			l.Fatal(errors.New("unknown DATA_MODEL_VERSION"))

		}
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(resp)
		if err != nil {
			l.With("error", err).Info("failed to write response")
		}
	}
}

func ec2Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	logger.Debug("calling ec2Handler ")
	userIP := getIPFromRequest(r)
	if userIP == "" {
		return
	}

	metrics.MetadataRequests.Inc()
	l := logger.With("userIP", userIP)
	l.Info("got ip from request")
	hw, err := getByIP(context.Background(), hegelServer, userIP) // returns hardware data as []byte
	if err != nil {
		metrics.Errors.WithLabelValues("metadata", "lookup").Inc()
		l.With("error", err).Info("failed to get hardware by ip")
		w.WriteHeader(http.StatusInternalServerError)
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

	var resp []byte
	resp, err = filterMetadata(hw, filter)
	if err != nil {
		l.With("error", err).Info("failed to filter metadata")
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(resp)
	if err != nil {
		l.With("error", err).Info("failed to write response")
	}
}

// processEC2Query returns either a specific filter (used to parse hardware data for the value of a specific field),
// or a comma-separated list of metadata items (to be printed)
func processEC2Query(url string) (string, error) {
	query := strings.TrimRight(strings.TrimPrefix(url, "/2009-04-04"), "/") // remove base pattern and trailing slash

	filter, ok := ec2Filters[query]
	if !ok {
		return "", errors.New("invalid metadata item")
	}

	return filter, nil
}

func getIPFromRequest(r *http.Request) string {
	IPAddress := r.RemoteAddr
	if strings.ContainsRune(IPAddress, ':') {
		IPAddress, _, _ = net.SplitHostPort(IPAddress)
	}
	return IPAddress
}
