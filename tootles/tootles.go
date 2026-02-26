// package tootles is the code for serving metadata (ec2 style, etc).
// Useful for Cloud-init integration.
package tootles

import (
	"fmt"
	"net/http"

	"dario.cat/mergo"
	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/ec2"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/hack"
)

type Config struct {
	BackendEc2       ec2.Client
	BackendHack      hack.Client
	DebugMode        bool
	InstanceEndpoint bool
}

func NewConfig(c Config) *Config {
	defaults := &Config{
		DebugMode: false,
	}

	if err := mergo.Merge(defaults, &c); err != nil {
		panic(fmt.Sprintf("failed to merge config: %v", err))
	}

	return defaults
}

// EC2MetadataHandler returns an http.Handler that serves EC2-style metadata at
// /2009-04-04/... and optionally /tootles/instanceID/:instanceID/2009-04-04/...
func (c *Config) EC2MetadataHandler() http.Handler {
	if !c.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	fe := ec2.New(c.BackendEc2, c.InstanceEndpoint)
	fe.Configure(router)

	return router
}

// HackMetadataHandler returns an http.Handler that serves the /metadata endpoint
// for the rootio hub action.
func (c *Config) HackMetadataHandler() http.Handler {
	if !c.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	hack.Configure(router, c.BackendHack)

	return router
}
