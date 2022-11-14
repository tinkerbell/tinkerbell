package ginutil

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// TrailingSlashRouteHelper wraps a gin.IRouter and ensures every route registered with GET
// has a corresponding alternate route with or without, dependent on the endpoint being registered,
// a trailing slash.
type TrailingSlashRouteHelper struct {
	gin.IRouter
}

// GET overrides the internal gin.IRouter GET. It interrogates endpoint for a trailing slash. If
// no trailing slash is present, it registers the endpoint and a corresponding endpoint with a
// trailing slash using the same handler. If it does end in a trailing slash, it does the inverse.
func (r TrailingSlashRouteHelper) GET(endpoint string, handler ...gin.HandlerFunc) gin.IRoutes {
	// Determine if the alternate endpoint should end with a slash or have it stripped.
	// This ensures we don't have routes such as
	// 		/2009-04-04/meta-data/instance-id/
	// 		/2009-04-04/meta-data/instance-id//
	var alternateEndpoint string
	if strings.HasSuffix(endpoint, "/") {
		alternateEndpoint = strings.TrimSuffix(endpoint, "/")
	} else {
		alternateEndpoint = endpoint + "/"
	}

	return r.IRouter.
		GET(endpoint, handler...).
		GET(alternateEndpoint, handler...)
}
