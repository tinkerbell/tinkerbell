package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"

	"github.com/packethost/cacher/protos/cacher"
	"github.com/packethost/hegel/grpc/hegel"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

//go:generate protoc -I grpc/protos grpc/protos/hegel.proto --go_out=plugins=grpc:grpc/hegel

type exportedHardware struct {
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

var active_subscription = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "hegel_active_subscriptions",
	Help: "Number of active hegel subscribers",
})

var active_cacher_subscription = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "hegel_active_cacher_subscriptions",
	Help: "Number of active hegel subscriptions to cacher",
})

func exportHardware(hw []byte) ([]byte, error) {
	exported := &exportedHardware{}
	err := json.Unmarshal(hw, exported)
	if err != nil {
		return nil, err
	}
	return json.Marshal(exported)
}

func (eh *exportedHardware) UnmarshalJSON(b []byte) error {
	type ehj exportedHardware
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
	*eh = exportedHardware(tmp)
	return nil
}

func (s *server) Get(ctx context.Context, in *hegel.GetRequest) (*hegel.GetResponse, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, errors.New("could not get peer info from client")
	}

	s.log.With("client", p.Addr, "op", "get").Info()
	hw, err := s.cacherClient.ByIP(ctx, &cacher.GetRequest{
		IP: p.Addr.(*net.TCPAddr).IP.String(),
	})

	if err != nil {
		return nil, err
	}

	if hw == nil {
		return nil, errors.New("could not find hardware")
	}

	ehw, err := exportHardware([]byte(hw.JSON))
	if err != nil {
		return nil, err
	}

	return &hegel.GetResponse{
		JSON: string(ehw),
	}, nil
}

func (s *server) Subscribe(in *hegel.SubscribeRequest, stream hegel.Hegel_SubscribeServer) error {
	active_subscription.Inc()
	defer active_subscription.Dec()
	p, ok := peer.FromContext(stream.Context())
	if !ok {
		return errors.New("could not get peer info from client")
	}

	s.log.With("client", p.Addr, "op", "subscribe").Info()
	hw, err := s.cacherClient.ByIP(stream.Context(), &cacher.GetRequest{
		IP: p.Addr.(*net.TCPAddr).IP.String(),
	})

	if err != nil {
		s.log.Error(err)
		return err
	}

	hwJSON := make(map[string]interface{})
	err = json.Unmarshal([]byte(hw.JSON), &hwJSON)
	if err != nil {
		return err
	}

	hwID := hwJSON["id"]

	ctx, cancel := context.WithCancel(stream.Context())
	watch, err := s.cacherClient.Watch(ctx, &cacher.GetRequest{
		ID: hwID.(string),
	})

	if err != nil {
		cancel()
		return err
	}

	errs := make(chan error, 1)
	ehws := make(chan []byte, 1)
	go func() {
		active_cacher_subscription.Inc()
		defer active_cacher_subscription.Dec()
		for {
			hw, err := watch.Recv()
			if err != nil {
				if err == io.EOF {
					err = status.Error(codes.OK, "stream ended")
				}
				errs <- err
				close(ehws)
				return
			}

			ehw, err := exportHardware([]byte(hw.JSON))
			if err != nil {
				errs <- err
				close(ehws)
				return
			}

			ehws <- ehw
		}
	}()
	go func() {
		l := s.log.With("client", p.Addr, "op", "send")
		for ehw := range ehws {
			l.Info()
			err = stream.Send(&hegel.SubscribeResponse{
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
	if err = <-errs; status.Code(err) != codes.OK && retErr == nil {
		retErr = err
	}
	return retErr
}
