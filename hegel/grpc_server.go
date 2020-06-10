package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"

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

type exportedHardware interface{}

type exportedHardwareCacher struct {
	ID                                 string                   `json:"id"`
	State                              string                   `json:"state"`
	Instance                           interface{}              `json:"instance"`
	PreinstalledOperatingSystemVersion interface{}              `json:"preinstalled_operating_system_version"`
	NetworkPorts                       []map[string]interface{} `json:"network_ports"`
	PlanSlug                           string                   `json:"plan_slug"`
	Facility                           string                   `json:"facility_code"`
	Hostname                           string                   `json:"hostname"`
	BondingMode                        int                      `json:"bonding_mode"`
}

type exportedHardwareTinkerbell struct {
	ID       string   `json:"id"`
	Metadata Metadata `json:"metadata"`
}

type Metadata struct {
	State        string      `json:"state"`
	BondingMode  int         `json:"bonding_mode"`
	Manufacturer interface{} `json:"manufacturer"`
	Instance     interface{} `json:"instance"`
	Custom       interface{} `json:"custom"`
	Facility     interface{} `json:"facility"`
}

// exportedHardware transforms hardware that is returned from cacher/tink into what we want to expose to clients
func exportHardware(hw []byte) ([]byte, error) {
	var exported exportedHardware

	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	switch dataModelVersion {
	case "1":
		exported = &exportedHardwareTinkerbell{}
	default:
		exported = &exportedHardwareCacher{}
	}

	err := json.Unmarshal(hw, exported)
	if err != nil {
		return nil, err
	}
	return json.Marshal(exported)
}

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
	// todo add tink
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, errors.New("could not get peer info from client")
	}
	s.log.With("client", p.Addr, "op", "get").Info()
	userIP := p.Addr.(*net.TCPAddr).IP.String()

	ehw, err := getByIP(ctx, s, userIP)
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
		//tc := s.hardwareClient.(tink.HardwareServiceClient)
		hw, err := s.hardwareClient.ByIP(stream.Context(), &tink.GetRequest{
			Ip: ip,
		})

		if err != nil {
			return initError(err)
		}

		ctx, cancel = context.WithCancel(stream.Context())
		watch, err = s.hardwareClient.Watch(ctx, &tink.GetRequest{
			Id: hw.(tink.Hardware).Id,
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
		err = json.Unmarshal([]byte(hw.(cacher.Hardware).JSON), &hwJSON)
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
				hw, err = json.Marshal(resp)
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

		if resp == nil {
			return nil, errors.New("could not find hardware")
		}

		hw, err = json.Marshal(resp)
		if err != nil {
			return nil, errors.New("could not marshal hardware")
		}
	default:
		req := &cacher.GetRequest{
			IP: userIP,
		}
		resp, err := s.hardwareClient.ByIP(ctx, req)

		if err != nil {
			return nil, err
		}

		if resp == nil {
			return nil, errors.New("could not find hardware")
		}

		hw = []byte(resp.(*cacher.Hardware).JSON)
	}

	ehw, err := exportHardware(hw)
	if err != nil {
		return nil, err
	}
	return ehw, nil
}
