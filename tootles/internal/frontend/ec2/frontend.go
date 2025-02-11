package ec2

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/data"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/ec2/internal/staticroute"
	"github.com/tinkerbell/tinkerbell/tootles/internal/ginutil"
	"github.com/tinkerbell/tinkerbell/tootles/internal/http/httperror"
	"github.com/tinkerbell/tinkerbell/tootles/internal/http/request"
)

// ErrInstanceNotFound indicates an instance could not be found for the given identifier.
var ErrInstanceNotFound = errors.New("instance not found")

// Client is a backend for retrieving EC2 Instance data.
type Client interface {
	// GetEC2Instance retrieves an Instance associated with ip. If no Instance can be
	// found, it should return ErrInstanceNotFound.
	GetEC2Instance(_ context.Context, ip string) (data.Ec2Instance, error)
}

// Frontend is an EC2 HTTP API frontend. It is responsible for configuring routers with handlers
// for the AWS EC2 instance metadata API.
type Frontend struct {
	client Client
}

// New creates a new Frontend.
func New(client Client) Frontend {
	return Frontend{
		client: client,
	}
}

// Configure configures router with the supported AWS EC2 instance metadata API endpoints.
//
// TODO(chrisdoherty4) Document unimplemented endpoints.
func (f Frontend) Configure(router gin.IRouter) {
	// Setup the 2009-04-04 API path prefix and use a trailing slash route helper to patch
	// equivalent trailing slash routes.
	v20090404 := ginutil.TrailingSlashRouteHelper{IRouter: router.Group("/2009-04-04")}

	dataEndpointBinder := func(router gin.IRouter, endpoint string, filter filterFunc) {
		router.GET(endpoint, func(ctx *gin.Context) {
			instance, err := f.getInstance(ctx, ctx.Request)
			if err != nil {
				// If there's an error containing an http status code, use that status code else
				// assume its an internal server error.
				var httpErr *httperror.E
				if errors.As(err, &httpErr) {
					_ = ctx.AbortWithError(httpErr.StatusCode, err)
				} else {
					_ = ctx.AbortWithError(http.StatusInternalServerError, err)
				}

				return
			}

			ctx.String(http.StatusOK, filter(instance))
		})
	}

	// Create a static route builder that we can add all data routes to which are the basis for
	// all static routes.
	staticRoutes := staticroute.NewBuilder()

	// Configure all dynamic routes. Dynamic routes are anything that requires retrieving a specific
	// instance and returning data from it.
	for _, r := range dataRoutes {
		dataEndpointBinder(v20090404, r.Endpoint, r.Filter)
		staticRoutes.FromEndpoint(r.Endpoint)
	}

	staticEndpointBinder := func(router gin.IRouter, endpoint string, childEndpoints []string) {
		router.GET(endpoint, func(ctx *gin.Context) {
			ctx.String(http.StatusOK, join(childEndpoints))
		})
	}

	for _, r := range staticRoutes.Build() {
		staticEndpointBinder(v20090404, r.Endpoint, r.Children)
	}
}

// getInstance is a framework agnostic method for retrieving Instance data based on a remote
// address.
func (f Frontend) getInstance(ctx context.Context, r *http.Request) (data.Ec2Instance, error) {
	ip, err := request.RemoteAddrIP(r)
	if err != nil {
		return data.Ec2Instance{}, httperror.New(http.StatusBadRequest, "invalid remote addr")
	}

	instance, err := f.client.GetEC2Instance(ctx, ip)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			return data.Ec2Instance{}, httperror.New(http.StatusNotFound, "no hardware found for source ip")
		}

		// TODO(chrisdoherty4) What happens when multiple Instance could be returned? What
		// is the behavior of GetEC2Instance?
		return data.Ec2Instance{}, httperror.Wrap(http.StatusInternalServerError, err)
	}

	return instance, nil
}

func join(v []string) string {
	return strings.Join(v, "\n")
}
