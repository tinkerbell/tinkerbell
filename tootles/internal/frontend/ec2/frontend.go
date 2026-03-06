package ec2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/ec2/internal/staticroute"
	"github.com/tinkerbell/tinkerbell/tootles/internal/ginutil"
	"github.com/tinkerbell/tinkerbell/tootles/internal/http/httperror"
	"github.com/tinkerbell/tinkerbell/tootles/internal/http/request"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrInstanceNotFound indicates an instance could not be found for the given identifier.
var ErrInstanceNotFound = errors.New("instance not found")

// Client is a backend for retrieving EC2 Instance data.
type Client interface {
	// GetEC2Instance retrieves an Instance associated with ip. If no Instance can be
	// found, it should return ErrInstanceNotFound.
	GetEC2Instance(_ context.Context, ip string) (data.Ec2Instance, error)
	// GetEC2InstanceByInstanceID retrieves an Instance by its Metadata Instance ID. If no Instance can be
	// found, it should return ErrInstanceNotFound.
	GetEC2InstanceByInstanceID(_ context.Context, instanceID string) (data.Ec2Instance, error)
}

// Frontend is an EC2 HTTP API frontend. It is responsible for configuring routers with handlers
// for the AWS EC2 instance metadata API.
type Frontend struct {
	client           Client
	instanceEndpoint bool
}

// New creates a new Frontend.
func New(client Client, instanceEndpoint bool) Frontend {
	return Frontend{
		client:           client,
		instanceEndpoint: instanceEndpoint,
	}
}

// Configure configures router with the supported AWS EC2 instance metadata API endpoints.
//
// TODO(chrisdoherty4) Document unimplemented endpoints.
func (f Frontend) Configure(router gin.IRouter) {
	// Setup the 2009-04-04 API path prefix and use a trailing slash route helper to patch
	// equivalent trailing slash routes.
	v20090404 := ginutil.TrailingSlashRouteHelper{IRouter: router.Group("/2009-04-04")}
	v20090404viaInstanceID := ginutil.TrailingSlashRouteHelper{IRouter: router.Group("/tootles/instanceID/:instanceID/2009-04-04")}

	// Create a static route builder that we can add all data routes to which are the basis for
	// all static routes.
	staticRoutes := staticroute.NewBuilder()

	// Configure all dynamic routes. Dynamic routes are anything that requires retrieving a specific
	// instance and returning data from it.
	for _, r := range dataRoutes {
		v20090404.GET(r.Endpoint, func(ctx *gin.Context) {
			instance, getInstanceErr := f.getInstanceViaIP(ctx, ctx.Request)
			f.writeInstanceDataOrErrToHTTP(ctx, getInstanceErr, r.Filter(instance))
		})

		if f.instanceEndpoint {
			v20090404viaInstanceID.GET(r.Endpoint, func(ctx *gin.Context) {
				instance, getInstanceErr := f.getInstanceViaInstanceID(ctx)
				f.writeInstanceDataOrErrToHTTP(ctx, getInstanceErr, r.Filter(instance))
			})
		}

		staticRoutes.FromEndpoint(r.Endpoint)
	}

	// Network interface attribute names exposed per MAC address.
	networkInterfaceAttributes := []string{"gateway", "local-ipv4", "mac", "netmask"}

	staticEndpointBinder := func(router ginutil.TrailingSlashRouteHelper, endpoint string, childEndpoints []string) {
		router.GET(endpoint, func(ctx *gin.Context) {
			ctx.String(http.StatusOK, join(childEndpoints))
		})
	}

	// Add network interface paths to the static route builder so parent listings include
	// "network/", "interfaces/", and "macs/" with trailing slashes indicating navigability.
	// We use a placeholder child under macs so the builder recognizes macs as a parent.
	// The generated static route for /meta-data/network/interfaces/macs is skipped below
	// because the MAC listing is handled by a dynamic handler.
	for _, attr := range networkInterfaceAttributes {
		staticRoutes.FromEndpoint("/meta-data/network/interfaces/macs/_/" + attr)
	}

	for _, r := range staticRoutes.Build() {
		// Skip the placeholder route - the MAC listing is dynamic (per-instance).
		if r.Endpoint == "/meta-data/network/interfaces/macs/_" || r.Endpoint == "/meta-data/network/interfaces/macs" {
			continue
		}
		staticEndpointBinder(v20090404, r.Endpoint, r.Children)
		if f.instanceEndpoint {
			staticEndpointBinder(v20090404viaInstanceID, r.Endpoint, r.Children)
		}
	}

	// Network interface dynamic routes.
	// These follow the EC2 convention: /meta-data/network/interfaces/macs/<mac>/<attribute>

	// List all MAC addresses.
	macListHandler := func(getInstance func(*gin.Context) (data.Ec2Instance, error)) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			instance, err := getInstance(ctx)
			if err != nil {
				f.writeInstanceDataOrErrToHTTP(ctx, err, "")
				return
			}
			var macs []string
			for _, iface := range instance.Metadata.Interfaces {
				macs = append(macs, iface.MAC+"/")
			}
			ctx.String(http.StatusOK, join(macs))
		}
	}

	// List available attributes for a MAC.
	macAttrListHandler := func(ctx *gin.Context) {
		ctx.String(http.StatusOK, join(networkInterfaceAttributes))
	}

	// Return a specific attribute for a MAC.
	macAttrHandler := func(getInstance func(*gin.Context) (data.Ec2Instance, error), attr string) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			instance, err := getInstance(ctx)
			if err != nil {
				f.writeInstanceDataOrErrToHTTP(ctx, err, "")
				return
			}
			mac := ctx.Param("mac")
			iface, found := findInterface(instance.Metadata.Interfaces, mac)
			if !found {
				ctx.String(http.StatusNotFound, "interface not found")
				return
			}
			var value string
			switch attr {
			case "mac":
				value = iface.MAC
			case "local-ipv4":
				value = iface.IP
			case "netmask":
				value = iface.Netmask
			case "gateway":
				value = iface.Gateway
			}
			ctx.String(http.StatusOK, value)
		}
	}

	getInstanceViaIPFunc := func(ctx *gin.Context) (data.Ec2Instance, error) {
		return f.getInstanceViaIP(ctx, ctx.Request)
	}
	getInstanceViaInstanceIDFunc := func(ctx *gin.Context) (data.Ec2Instance, error) {
		return f.getInstanceViaInstanceID(ctx)
	}

	// Register network interface routes for IP-based access.
	v20090404.GET("/meta-data/network/interfaces/macs", macListHandler(getInstanceViaIPFunc))
	v20090404.GET("/meta-data/network/interfaces/macs/:mac", macAttrListHandler)
	for _, attr := range networkInterfaceAttributes {
		v20090404.GET("/meta-data/network/interfaces/macs/:mac/"+attr, macAttrHandler(getInstanceViaIPFunc, attr))
	}

	// Register network interface routes for instance ID-based access.
	if f.instanceEndpoint {
		v20090404viaInstanceID.GET("/meta-data/network/interfaces/macs", macListHandler(getInstanceViaInstanceIDFunc))
		v20090404viaInstanceID.GET("/meta-data/network/interfaces/macs/:mac", macAttrListHandler)
		for _, attr := range networkInterfaceAttributes {
			v20090404viaInstanceID.GET("/meta-data/network/interfaces/macs/:mac/"+attr, macAttrHandler(getInstanceViaInstanceIDFunc, attr))
		}
	}
}

func findInterface(interfaces []data.NetworkInterface, mac string) (data.NetworkInterface, bool) {
	for _, iface := range interfaces {
		if iface.MAC == mac {
			return iface, true
		}
	}
	return data.NetworkInterface{}, false
}

// Shared across IP and instanceID-based routes.
func (f Frontend) writeInstanceDataOrErrToHTTP(ctx *gin.Context, getInstanceErr error, filteredInstanceData string) {
	if getInstanceErr != nil {
		// If there's an error containing an http status code, use that status code else assume it is an internal server error.
		var httpErr *httperror.E
		if errors.As(getInstanceErr, &httpErr) {
			_ = ctx.AbortWithError(httpErr.StatusCode, getInstanceErr)
		} else {
			_ = ctx.AbortWithError(http.StatusInternalServerError, getInstanceErr)
		}

		return
	}
	// Simply output the filtered instance data with a 200 OK status code.
	ctx.String(http.StatusOK, filteredInstanceData)
}

// getInstanceViaIP is a framework-agnostic method for retrieving Instance data based on a remote
// address. Normal IP based lookup. SNAT, proxies, externalTrafficPolicy:Cluster, possibly
// misconfigured X-Forwarded-For headers, etc. are all in play here.
func (f Frontend) getInstanceViaIP(ctx context.Context, r *http.Request) (data.Ec2Instance, error) {
	ip, err := request.RemoteAddrIP(r)
	if err != nil {
		return data.Ec2Instance{}, httperror.New(http.StatusBadRequest, "invalid remote addr")
	}

	instance, err := f.client.GetEC2Instance(ctx, ip)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) || apierrors.IsNotFound(err) {
			return data.Ec2Instance{}, httperror.New(http.StatusNotFound, fmt.Sprintf("no hardware found for source ip: %s", ip))
		}

		// TODO(chrisdoherty4) What happens when multiple Instance could be returned? What
		// is the behavior of GetEC2Instance?
		return data.Ec2Instance{}, httperror.Wrap(http.StatusInternalServerError, err)
	}

	return instance, nil
}

// getInstanceViaInstanceID is a gin-specific method for retrieving Instance data based on the instance ID included in the request path.
// It is currently gin-specific because it depends on *gin.Context and ctx.Param("instanceID"), unlike getInstanceViaIP.
func (f Frontend) getInstanceViaInstanceID(ctx *gin.Context) (data.Ec2Instance, error) {
	instanceID := ctx.Param("instanceID")
	if strings.TrimSpace(instanceID) == "" {
		return data.Ec2Instance{}, httperror.New(http.StatusNotFound, "instance ID parameter is empty or invalid")
	}

	instance, err := f.client.GetEC2InstanceByInstanceID(ctx, instanceID)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) || apierrors.IsNotFound(err) {
			return data.Ec2Instance{}, httperror.New(http.StatusNotFound, fmt.Sprintf("no hardware found for instanceID: '%s'", instanceID))
		}
		return data.Ec2Instance{}, httperror.Wrap(http.StatusInternalServerError, err)
	}

	return instance, nil
}

func join(v []string) string {
	return strings.Join(v, "\n")
}
