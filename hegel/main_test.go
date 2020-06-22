package main

import (
	"os"
	"testing"

	"github.com/packethost/hegel/metrics"
	"github.com/packethost/pkg/log"
)

func TestMain(m *testing.M) {
	hegelServer = &server{
		log: logger,
	}
	os.Setenv("PACKET_ENV", "test")
	os.Setenv("PACKET_VERSION", "ignored")
	os.Setenv("ROLLBAR_TOKEN", "ignored")

	l, _ := log.Init("github.com/packethost/hegel")
	logger = l.Package("main")
	metrics.Init(l)

	os.Exit(m.Run())
}
