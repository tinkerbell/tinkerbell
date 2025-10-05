package nocloud

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/tootles/internal/http/httperror"
	"github.com/tinkerbell/tinkerbell/tootles/internal/http/request"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrInstanceNotFound indicates an instance could not be found for the given identifier.
var ErrInstanceNotFound = errors.New("instance not found")

// Client is a backend for retrieving NoCloud Instance data.
type Client interface {
	// GetNoCloudInstance retrieves an Instance associated with ip. If no Instance can be
	// found, it should return ErrInstanceNotFound.
	GetNoCloudInstance(_ context.Context, ip string) (data.NoCloudInstance, error)
}

// Frontend is a NoCloud HTTP API frontend. It is responsible for configuring routers with handlers
// for the NoCloud metadata API.
type Frontend struct {
	client Client
}

// New creates a new Frontend.
func New(client Client) Frontend {
	return Frontend{
		client: client,
	}
}

// Configure configures router with the supported NoCloud metadata API endpoints.
func (f Frontend) Configure(router gin.IRouter) {
	// Configure NoCloud endpoints directly under the root path
	router.GET("/meta-data", f.metaDataHandler)
	router.GET("/user-data", f.userDataHandler)
	router.GET("/network-config", f.networkConfigHandler)
}

// metaDataHandler handles the /meta-data endpoint.
func (f Frontend) metaDataHandler(ctx *gin.Context) {
	instance, err := f.getInstance(ctx, ctx.Request)
	if err != nil {
		var httpErr *httperror.E
		if errors.As(err, &httpErr) {
			_ = ctx.AbortWithError(httpErr.StatusCode, err)
		} else {
			_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		}
		return
	}

	// Format metadata as YAML-like text output
	output := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s", instance.Metadata.InstanceID, instance.Metadata.LocalHostname)

	ctx.Header("Content-Type", "text/plain")
	ctx.String(http.StatusOK, output)
}

// userDataHandler handles the /user-data endpoint.
func (f Frontend) userDataHandler(ctx *gin.Context) {
	instance, err := f.getInstance(ctx, ctx.Request)
	if err != nil {
		var httpErr *httperror.E
		if errors.As(err, &httpErr) {
			_ = ctx.AbortWithError(httpErr.StatusCode, err)
		} else {
			_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		}
		return
	}

	if instance.Userdata == "" {
		_ = ctx.AbortWithError(http.StatusNotFound, errors.New("user-data not found"))
		return
	}

	ctx.Header("Content-Type", "text/plain")
	ctx.String(http.StatusOK, instance.Userdata)
}

// networkConfigHandler handles the /network-config endpoint.
func (f Frontend) networkConfigHandler(ctx *gin.Context) {
	instance, err := f.getInstance(ctx, ctx.Request)
	if err != nil {
		var httpErr *httperror.E
		if errors.As(err, &httpErr) {
			_ = ctx.AbortWithError(httpErr.StatusCode, err)
		} else {
			_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		}
		return
	}

	if instance.NetworkConfig == nil {
		// Return fallback DHCP configuration
		fallbackConfig := map[string]interface{}{
			"version": 1,
			"config": []interface{}{
				map[string]interface{}{
					"type": "physical",
					"name": "eno1",
					"subnets": []interface{}{
						map[string]interface{}{
							"type": "dhcp",
						},
					},
				},
			},
		}

		yamlData, err := yaml.Marshal(fallbackConfig)
		if err != nil {
			_ = ctx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to marshal fallback config: %w", err))
			return
		}

		ctx.Header("Content-Type", "text/yaml")
		ctx.String(http.StatusOK, string(yamlData))
		return
	}

	yamlData, err := yaml.Marshal(instance.NetworkConfig)
	if err != nil {
		_ = ctx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to marshal network config: %w", err))
		return
	}

	ctx.Header("Content-Type", "text/yaml")
	ctx.String(http.StatusOK, string(yamlData))
}

// getInstance is a framework agnostic method for retrieving Instance data based on a remote
// address.
func (f Frontend) getInstance(ctx context.Context, r *http.Request) (data.NoCloudInstance, error) {
	ip, err := request.RemoteAddrIP(r)
	if err != nil {
		return data.NoCloudInstance{}, httperror.New(http.StatusBadRequest, "invalid remote addr")
	}

	instance, err := f.client.GetNoCloudInstance(ctx, ip)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) || apierrors.IsNotFound(err) {
			return data.NoCloudInstance{}, httperror.New(http.StatusNotFound, fmt.Sprintf("no hardware found for source ip: %s", ip))
		}

		return data.NoCloudInstance{}, httperror.Wrap(http.StatusInternalServerError, err)
	}

	return instance, nil
}
