package ec2

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/packethost/pkg/log"
	"github.com/tinkerbell/hegel/internal/http/httperror"
	"github.com/tinkerbell/hegel/internal/http/request"
)

// ErrInstanceNotFound indicates an instance could not be found for the given identifier.
var ErrInstanceNotFound = errors.New("instance not found")

// Client is a backend for retrieving EC2 Instance data.
type Client interface {
	// GetEC2Instance retrieves an Instance associated with ip. If no Instance can be
	// found, it should return ErrInstanceNotFound.
	GetEC2Instance(_ context.Context, ip string) (Instance, error)
}

// Frontend is an EC2 HTTP API frontend. It is responsible for configuring routers with handlers
// for the AWS EC2 instance metadata API.
type Frontend struct {
	log    log.Logger
	client Client
}

// New creates a new Frontend.
func New(logger log.Logger, client Client) Frontend {
	return Frontend{
		log:    logger,
		client: client,
	}
}

// Configure configures router with the supported AWS EC2 instance metadata API endpoints.
//
// TODO(chrisdoherty4) Document unimplemented endpoints.
func (f Frontend) Configure(router *gin.Engine) {
	// Setup the 2009-04-04 API path prefix.
	v20090404 := router.Group("/2009-04-04")

	dynamicEndpointBinder := func(router *gin.RouterGroup, endpoint string, filter filterFunc) {
		router.GET(endpoint, func(ctx *gin.Context) {
			instance, err := f.getInstance(ctx, ctx.Request)
			if err != nil {
				// If there's an error containing an http status code, use that status code else
				// assume its an internal server error.
				var httpErr *httperror.E
				if errors.As(err, &httpErr) {
					_ = ctx.AbortWithError(httpErr.StatusCode, errors.New("foo bar"))
				} else {
					_ = ctx.AbortWithError(http.StatusInternalServerError, err)
				}

				return
			}

			ctx.String(http.StatusOK, filter(instance))
		})
	}

	// Configure all dynamic routes. Dynamic routes are anything that requires retrieving a specific
	// instance and returning data from it.
	for _, route := range dynamicRoutes {
		dynamicEndpointBinder(v20090404, route.Endpoint, route.Filter)
	}

	staticEndpointBinder := func(router *gin.RouterGroup, endpoint string, childEndpoints []string) {
		router.GET(endpoint, func(ctx *gin.Context) {
			ctx.String(http.StatusOK, join(childEndpoints))
		})
	}

	for _, route := range staticRoutes {
		children := make([]string, len(route.ChildEndpoints))
		copy(children, route.ChildEndpoints)
		sort.Strings(children)
		staticEndpointBinder(v20090404, route.Endpoint, children)
	}
}

// getInstance is a framework agnostic method for retrieving Instance data based on a remote
// address.
func (f Frontend) getInstance(ctx context.Context, r *http.Request) (Instance, error) {
	ip, err := request.RemoteAddrIP(r)
	if err != nil {
		f.log.Info("Invalid remote address", "err", err)
		return Instance{}, httperror.New(http.StatusBadRequest, "invalid remote addr")
	}

	instance, err := f.client.GetEC2Instance(ctx, ip)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			return Instance{}, httperror.New(http.StatusNotFound, "no hardware found for source ip")
		}

		// TODO(chrisdoherty4) What happens when multiple Instance could be returned? What
		// is the behavior of GetEC2Instance?
		return Instance{}, httperror.Wrap(http.StatusInternalServerError, err)
	}

	return instance, nil
}

func join(v []string) string {
	return strings.Join(v, "\n")
}
