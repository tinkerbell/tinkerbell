package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/itchyny/gojq"
	"github.com/tinkerbell/tink/util"

	"github.com/packethost/cacher/protos/cacher"
	"github.com/packethost/hegel/grpc/hegel"
	"github.com/packethost/hegel/metrics"
	tink "github.com/tinkerbell/tink/protos/hardware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

//go:generate protoc -I grpc/protos grpc/protos/hegel.proto --go_out=plugins=grpc:grpc/hegel

type watchClient interface{}

// exportedHardwareCacher is the structure in which hegel returns to clients using the old cacher data model
// exposes only certain fields of the hardware data returned by cacher
type exportedHardwareCacher struct {
	ID                                 string                   `json:"id"`
	Arch                               string                   `json:"arch"`
	State                              string                   `json:"state"`
	EFIBoot                            bool                     `json:"efi_boot"`
	Instance                           instance                 `json:"instance"`
	PreinstalledOperatingSystemVersion interface{}              `json:"preinstalled_operating_system_version"`
	NetworkPorts                       []map[string]interface{} `json:"network_ports"`
	PlanSlug                           string                   `json:"plan_slug"`
	Facility                           string                   `json:"facility_code"`
	Hostname                           string                   `json:"hostname"`
	BondingMode                        int                      `json:"bonding_mode"`
}

type instance struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Hostname string `json:"hostname"`
	AllowPXE bool   `json:"allow_pxe"`
	Rescue   bool   `json:"rescue"`

	OS       operatingSystem `json:"operating_system_version"`
	UserData string          `json:"userdata,omitempty"`

	CryptedRootPassword string `json:"crypted_root_password,omitempty"`

	Storage      storage  `json:"storage,omitempty"`
	SSHKeys      []string `json:"ssh_keys,omitempty"`
	NetworkReady bool     `json:"network_ready,omitempty"`
}

type operatingSystem struct {
	Slug     string `json:"slug"`
	Distro   string `json:"distro"`
	Version  string `json:"version"`
	ImageTag string `json:"image_tag"`
	OsSlug   string `json:"os_slug"`
}

type disk struct {
	Device    string       `json:"device"`
	WipeTable bool         `json:"wipeTable,omitempty"`
	Paritions []*partition `json:"partitions,omitempty"`
}

type file struct {
	Path     string `json:"path"`
	Contents string `json:"contents,omitempty"`
	Mode     int    `json:"mode,omitempty"`
	UID      int    `json:"uid,omitempty"`
	GID      int    `json:"gid,omitempty"`
}

type filesystem struct {
	Mount struct {
		Device string             `json:"device"`
		Format string             `json:"format"`
		Files  []*file            `json:"files,omitempty"`
		Create *filesystemOptions `json:"create,omitempty"`
		Point  string             `json:"point"`
	} `json:"mount"`
}

type filesystemOptions struct {
	Force   bool     `json:"force,omitempty"`
	Options []string `json:"options,omitempty"`
}

type partition struct {
	Label    string      `json:"label,omitempty"`
	Number   int         `json:"number,omitempty"`
	Size     intOrString `json:"size,omitempty"`
	Start    int         `json:"start,omitempty"`
	TypeGUID string      `json:"typeGuid,omitempty"`
}

type raid struct {
	Name    string   `json:"name"`
	Level   string   `json:"level"`
	Devices []string `json:"devices"`
	Spares  int      `json:"spares,omitempty"`
}

type storage struct {
	Disks       []*disk       `json:"disks,omitempty"`
	RAID        []*raid       `json:"raid,omitempty"`
	Filesystems []*filesystem `json:"filesystems,omitempty"`
}

type intOrString int

// UnmarshalJSON implements the json.Unmarshaler interface for custom unmarshalling of IntOrString
func (ios *intOrString) UnmarshalJSON(b []byte) error {
	if b[0] != '"' {
		return json.Unmarshal(b, (*int)(ios))
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	i, err := convertSuffix(s)
	if err != nil {
		return err
	}
	*ios = intOrString(i)
	return nil
}

// convertSuffix converts a string of "<size><suffix>" (e.g. "123kb") into its equivalent size in bytes
func convertSuffix(s string) (int, error) {
	if s == "" {
		return 0, nil
	}

	suffixes := map[string]float64{"": 0, "b": 0, "k": 1, "kb": 1, "m": 2, "mb": 2, "g": 3, "gb": 3, "t": 4, "tb": 4}
	s = strings.ToLower(s)
	i := strings.TrimFunc(s, func(r rune) bool { // trims both ends of string s of anything not a number (e.g., "123 kb" -> "123" and "12k3b" -> "12k3")
		return !unicode.IsNumber(r)
	})
	size, err := strconv.Atoi(i)
	if err != nil {
		return -1, err
	}

	suf := strings.TrimFunc(s, func(r rune) bool { // trims both ends of string s of anything not a letter (e.g., "123 kb" -> "kb")
		return !unicode.IsLetter(r)
	})

	if pow, ok := suffixes[suf]; ok {
		res := size * int(math.Pow(1024, pow))
		return res, nil
	}
	return -1, errors.New("invalid suffix")
}

// exportedHardware transforms hardware that is returned from cacher into what we want to expose to clients
func exportHardware(hw []byte) ([]byte, error) {
	exported := &exportedHardwareCacher{}

	err := json.Unmarshal(hw, exported)
	if err != nil {
		return nil, err
	}
	return json.Marshal(exported)
}

func filterMetadata(hw []byte, filter string) ([]byte, error) {
	var result interface{}
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
		if err, ok := v.(error); ok {
			return nil, err
		}
		result = v
	}

	if resultString, ok := result.(string); ok { // if already a string, don't marshal
		return []byte(resultString), nil
	}
	if result != nil { // if nil, don't marshal (json.Marshal(nil) returns "null")
		return json.Marshal(result)
	}
	return nil, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface for custom unmarshalling of exportedHardwareCacher
func (eh *exportedHardwareCacher) UnmarshalJSON(b []byte) error {
	type ehj exportedHardwareCacher
	var tmp ehj
	err := json.Unmarshal(b, &tmp)
	if err != nil {
		return err
	}
	networkPorts := []map[string]interface{}{}
	for _, port := range tmp.NetworkPorts {
		if port["type"] == "data" {
			networkPorts = append(networkPorts, port)
		}
	}
	tmp.NetworkPorts = networkPorts
	*eh = exportedHardwareCacher(tmp)
	return nil
}

func (s *server) Get(ctx context.Context, in *hegel.GetRequest) (*hegel.GetResponse, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, errors.New("could not get peer info from client")
	}
	s.log.With("client", p.Addr, "op", "get").Info()
	userIP := p.Addr.(*net.TCPAddr).IP.String()

	hw, err := getByIP(ctx, s, userIP)
	if err != nil {
		return nil, err
	}
	ehw, err := exportHardware(hw)
	if err != nil {
		return nil, err
	}
	return &hegel.GetResponse{
		JSON: string(ehw),
	}, nil
}

func (s *server) Subscribe(in *hegel.SubscribeRequest, stream hegel.Hegel_SubscribeServer) error {
	metrics.TotalSubscriptions.Inc()
	metrics.Subscriptions.WithLabelValues("initializing").Inc()

	logger := s.log.With("op", "subscribe")

	initError := func(err error) error {
		logger.Error(err)
		metrics.Subscriptions.WithLabelValues("initializing").Dec()
		metrics.Errors.WithLabelValues("subscribe", "initializing").Inc()
		return err
	}

	p, ok := peer.FromContext(stream.Context())
	if !ok {
		return initError(errors.New("could not get peer info from client"))
	}

	ip := p.Addr.(*net.TCPAddr).IP.String()
	logger = logger.With("ip", ip, "client", p.Addr)

	logger.Info()

	var watch watchClient
	var ctx context.Context
	var cancel context.CancelFunc
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	switch dataModelVersion {
	case "1":
		hw, err := s.hardwareClient.ByIP(stream.Context(), &tink.GetRequest{
			Ip: ip,
		})

		if err != nil {
			return initError(err)
		}

		ctx, cancel = context.WithCancel(stream.Context())
		watch, err = s.hardwareClient.Watch(ctx, &tink.GetRequest{
			Id: hw.(*tink.Hardware).Id,
		})

		if err != nil {
			cancel()
			return initError(err)
		}
	default:
		hw, err := s.hardwareClient.ByIP(stream.Context(), &cacher.GetRequest{
			IP: ip,
		})

		if err != nil {
			return initError(err)
		}

		hwJSON := make(map[string]interface{})
		err = json.Unmarshal([]byte(hw.(*cacher.Hardware).JSON), &hwJSON)
		if err != nil {
			return initError(err)
		}

		hwID := hwJSON["id"]

		ctx, cancel = context.WithCancel(stream.Context())
		watch, err = s.hardwareClient.Watch(ctx, &cacher.GetRequest{
			ID: hwID.(string),
		})

		if err != nil {
			cancel()
			return initError(err)
		}
	}

	metrics.Subscriptions.WithLabelValues("initializing").Dec()
	metrics.Subscriptions.WithLabelValues("active").Inc()
	activeError := func(err error) error {
		if err == nil {
			return err
		}
		logger.Error(err)
		metrics.Subscriptions.WithLabelValues("active").Dec()
		metrics.Errors.WithLabelValues("subscribe", "active").Inc()
		return err
	}

	errs := make(chan error, 1)
	ehws := make(chan []byte, 1)
	go func() {
		for {
			var hw []byte
			dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
			switch dataModelVersion {
			case "1":
				wt := watch.(tink.HardwareService_WatchClient)
				resp, err := wt.Recv()
				if err != nil {
					if err == io.EOF {
						err = status.Error(codes.OK, "stream ended")
					}
					errs <- err
					close(ehws)
					return
				}
				hw, err = json.Marshal(util.HardwareWrapper{Hardware: resp})
				if err != nil {
					errs <- errors.New("could not marshal hardware")
					close(ehws)
					return
				}
			default:
				wc := watch.(cacher.Cacher_WatchClient)
				resp, err := wc.Recv()
				if err != nil {
					if err == io.EOF {
						err = status.Error(codes.OK, "stream ended")
					}
					errs <- err
					close(ehws)
					return
				}
				hw = []byte(resp.JSON)
			}

			ehw, err := exportHardware(hw)
			if err != nil {
				errs <- err
				close(ehws)
				return
			}

			ehws <- ehw
		}
	}()
	go func() {
		l := logger.With("op", "send")
		for ehw := range ehws {
			l.Info()
			err := stream.Send(&hegel.SubscribeResponse{
				JSON: string(ehw),
			})

			if err != nil {
				errs <- err
				cancel()
				return
			}
		}

	}()

	var retErr error
	if err := <-errs; status.Code(err) != codes.OK && retErr == nil {
		retErr = err
	}
	return activeError(retErr)
}

func getByIP(ctx context.Context, s *server, userIP string) ([]byte, error) {

	var hw []byte
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	switch dataModelVersion {
	case "1":
		req := &tink.GetRequest{
			Ip: userIP,
		}
		resp, err := s.hardwareClient.ByIP(ctx, req)

		if err != nil {
			return nil, err
		}

		hw, err = json.Marshal(util.HardwareWrapper{Hardware: resp.(*tink.Hardware)})
		if err != nil {
			return nil, errors.New("could not marshal hardware")
		}

		if string(hw) == "{}" {
			return nil, errors.New("could not find hardware")
		}

	default:
		req := &cacher.GetRequest{
			IP: userIP,
		}
		resp, err := s.hardwareClient.ByIP(ctx, req)

		if err != nil {
			return nil, err
		}

		hw = []byte(resp.(*cacher.Hardware).JSON)
		if string(hw) == "" {
			return nil, errors.New("could not find hardware")
		}
	}

	return hw, nil
}
