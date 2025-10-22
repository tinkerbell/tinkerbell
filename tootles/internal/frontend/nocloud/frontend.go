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
	// Configure NoCloud endpoints under /nocloud prefix
	router.GET("/nocloud/meta-data", f.metaDataHandler)
	router.GET("/nocloud/user-data", f.userDataHandler)
	router.GET("/nocloud/vendor-data", f.vendorDataHandler)
	router.GET("/nocloud/network-config", f.networkConfigHandler)
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

	// Build metadata map with all relevant fields
	metadata := map[string]interface{}{
		"instance-id":    instance.Metadata.InstanceID,
		"local-hostname": instance.Metadata.LocalHostname,
	}

	// Include optional fields if they are set
	if instance.Metadata.Hostname != "" {
		metadata["hostname"] = instance.Metadata.Hostname
	}
	if instance.Metadata.IQN != "" {
		metadata["iqn"] = instance.Metadata.IQN
	}
	if instance.Metadata.Plan != "" {
		metadata["plan"] = instance.Metadata.Plan
	}
	if instance.Metadata.Facility != "" {
		metadata["facility"] = instance.Metadata.Facility
	}
	if len(instance.Metadata.Tags) > 0 {
		metadata["tags"] = instance.Metadata.Tags
	}
	if len(instance.Metadata.PublicKeys) > 0 {
		metadata["public-keys"] = instance.Metadata.PublicKeys
	}
	if instance.Metadata.PublicIPv4 != "" {
		metadata["public-ipv4"] = instance.Metadata.PublicIPv4
	}
	if instance.Metadata.PublicIPv6 != "" {
		metadata["public-ipv6"] = instance.Metadata.PublicIPv6
	}
	if instance.Metadata.LocalIPv4 != "" {
		metadata["local-ipv4"] = instance.Metadata.LocalIPv4
	}

	yamlData, err := yaml.Marshal(metadata)
	if err != nil {
		_ = ctx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to marshal metadata: %w", err))
		return
	}

	ctx.Header("Content-Type", "text/plain")
	ctx.String(http.StatusOK, string(yamlData))
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

// vendorDataHandler handles the /vendor-data endpoint.
func (f Frontend) vendorDataHandler(ctx *gin.Context) {
	_, err := f.getInstance(ctx, ctx.Request)
	if err != nil {
		var httpErr *httperror.E
		if errors.As(err, &httpErr) {
			_ = ctx.AbortWithError(httpErr.StatusCode, err)
		} else {
			_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		}
		return
	}

	// Vendor data is not currently supported, return empty content with 200 OK
	ctx.Header("Content-Type", "text/plain")
	ctx.String(http.StatusOK, "")
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
		// Return empty configuration
		ctx.Header("Content-Type", "text/plain")
		ctx.String(http.StatusOK, "")
		return
	}

	yamlData, err := yaml.Marshal(instance.NetworkConfig)
	if err != nil {
		_ = ctx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to marshal network config: %w", err))
		return
	}

	ctx.Header("Content-Type", "text/plain")
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
