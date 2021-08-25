package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/pkg/errors"
	grpcserver "github.com/tinkerbell/hegel/grpc-server"
	"github.com/tinkerbell/hegel/metrics"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
	isHardwareClientAvailableMu.RLock()
	isHardwareClientAvailableTemp := isHardwareClientAvailable
	isHardwareClientAvailableMu.RUnlock()

	res := struct {
		GitRev                  string  `json:"git_rev"`
		Uptime                  float64 `json:"uptime"`
		Goroutines              int     `json:"goroutines"`
		HardwareClientAvailable bool    `json:"hardware_client_status"`
	}{
		GitRev:                  gitRev,
		Uptime:                  time.Since(startTime).Seconds(),
		Goroutines:              runtime.NumGoroutine(),
		HardwareClientAvailable: isHardwareClientAvailableTemp,
	}
	b, err := json.Marshal(&res)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	if !isHardwareClientAvailableTemp {
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
		hw, err := hegelServer.HardwareClient().ByIP(context.Background(), userIP)
		if err != nil {
			metrics.Errors.WithLabelValues("metadata", "lookup").Inc()
			l.With("error", err).Info("failed to get hardware by ip")
			w.WriteHeader(http.StatusNotFound)
			return
		}

		ehw, err := hw.Export()
		if err != nil {
			l.With("error", err).Info("failed to export hardware")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var resp []byte
		dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
		switch dataModelVersion {
		case "":
			// in cacher mode, the "filter" is the exportedHardwareCacher type
			// TODO (kdeng3849) figure out a way to remove the switch case
			resp = ehw
		case "1":
			resp, err = filterMetadata(ehw, filter)
			if err != nil {
				l.With("error", err).Info("failed to filter metadata")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		default:
			l.Fatal(errors.New("unknown DATA_MODEL_VERSION"))
			w.WriteHeader(http.StatusInternalServerError)
			return

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
	hw, err := hegelServer.HardwareClient().ByIP(context.Background(), userIP)
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
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(resp)
	if err != nil {
		l.With("error", err).Info("failed to write response")
	}
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

func writeJSON(w http.ResponseWriter, status int, data interface{}) error {
	var body []byte
	body, err := json.Marshal(data)
	if err != nil {
		if status < 400 {
			return jsonError(w, http.StatusInternalServerError, err, "marshalling response")
		} else {
			status = 500
			logger.Error(err, "failed to marshal error")
			body = []byte(`{"error", {"comment": "Failed to marshal error"}}`)
		}
	}
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(body)
	return err
}

func jsonError(w http.ResponseWriter, status int, err error, msg string) error {
	logger.Error(err, msg)
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"error":   err.Error(),
			"comment": msg,
		},
	}
	return writeJSON(w, status, resp)
}

func handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	var getid string
	if strings.HasPrefix(r.URL.Path, "/subscriptions/") {
		getid = strings.TrimPrefix(r.URL.Path, "/subscriptions/")
	}
	hegelServer.SubLock().RLock()
	defer hegelServer.SubLock().RUnlock()
	var err error
	if getid == "" {
		err = writeJSON(w, http.StatusOK, hegelServer.Subscriptions)
	} else if sub, ok := hegelServer.Subscriptions()[getid]; ok {
		err = writeJSON(w, http.StatusOK, sub)
	} else {
		err = jsonError(w, http.StatusNotFound, fmt.Errorf("%s not found", getid), "item not found")
	}
	if err != nil {
		logger.Error(err)
	}
}

func buildSubscriberHandlers(hegelServer *grpcserver.Server) {
	handler := otelhttp.WithRouteTag("/subscriptions", http.HandlerFunc(handleSubscriptions))
	http.Handle("/subscriptions", handler)
	http.Handle("/subscriptions/", handler)
}
