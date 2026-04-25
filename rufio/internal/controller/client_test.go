package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bmc-toolbox/bmclib/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/jacobweinstock/registrar"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
)

type compatibleTestProvider struct {
	name       string
	compatible bool
	opened     bool
}

func (p *compatibleTestProvider) Name() string {
	return p.name
}

func (p *compatibleTestProvider) Compatible(context.Context) bool {
	return p.compatible
}

func (p *compatibleTestProvider) Open(context.Context) error {
	p.opened = true
	return nil
}

func TestPrepareClientForOpenFiltersIncompatibleDrivers(t *testing.T) {
	incompatible := &compatibleTestProvider{name: "dell", compatible: false}
	standard := &compatibleTestProvider{name: "ipmitool", compatible: true}
	preferred := &compatibleTestProvider{name: "gofish", compatible: true}
	client := newCompatibleTestClient(incompatible, standard, preferred)

	opts := &BMCOptions{
		ProviderOptions: &bmc.ProviderOptions{
			PreferredOrder: []bmc.ProviderName{"gofish"},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := prepareClientForOpen(ctx, client, opts); err != nil {
		t.Fatalf("prepare client for open: %v", err)
	}

	want := []string{"gofish", "ipmitool"}
	if diff := cmp.Diff(want, compatibleTestDriverNames(client.Registry.Drivers)); diff != "" {
		t.Fatalf("unexpected drivers (-want +got):\n%s", diff)
	}

	if err := client.Open(ctx); err != nil {
		t.Fatalf("open filtered client: %v", err)
	}
	if incompatible.opened {
		t.Fatal("incompatible driver was opened")
	}
	if !standard.opened || !preferred.opened {
		t.Fatal("compatible drivers were not opened")
	}
}

func TestPrepareClientForOpenErrorsWithoutCompatibleDrivers(t *testing.T) {
	client := newCompatibleTestClient(
		&compatibleTestProvider{name: "dell", compatible: false},
		&compatibleTestProvider{name: "ipmitool", compatible: false},
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := prepareClientForOpen(ctx, client, nil)
	if !errors.Is(err, errNoCompatibleDrivers) {
		t.Fatalf("expected no compatible drivers error, got %v", err)
	}
	if len(client.Registry.Drivers) != 0 {
		t.Fatalf("expected empty driver registry, got %d drivers", len(client.Registry.Drivers))
	}
}

func newCompatibleTestClient(providers ...*compatibleTestProvider) *bmclib.Client {
	registry := registrar.NewRegistry()
	for _, provider := range providers {
		registry.Register(provider.name, provider.name, nil, nil, provider)
	}

	return bmclib.NewClient("127.0.0.1", "username", "password", bmclib.WithRegistry(registry))
}

func compatibleTestDriverNames(drivers registrar.Drivers) []string {
	names := make([]string, 0, len(drivers))
	for _, driver := range drivers {
		names = append(names, driver.Name)
	}

	return names
}
