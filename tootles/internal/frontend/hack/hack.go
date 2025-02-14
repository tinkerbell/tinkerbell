/*
Package hack contains a frontend that provides a /metadata endpoint for the rootio hub action.
It is not intended to be long lived and will be removed as we migrate to exposing Hardware
data to Tinkerbell templates. In doing so, we can convert the rootio action to accept its inputs
via parameters instead of retrieving them from Tootles and subsequently delete this frontend.
*/
package hack

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/tootles/internal/http/request"
)

// Client is a backend for retrieving hack instance data.
type Client interface {
	GetHackInstance(ctx context.Context, ip string) (data.HackInstance, error)
}

// Configure configures router with a `/metadata` endpoint using client to retrieve instance data.
func Configure(router gin.IRouter, client Client) {
	router.GET("/metadata", func(ctx *gin.Context) {
		ip, err := request.RemoteAddrIP(ctx.Request)
		if err != nil {
			_ = ctx.AbortWithError(http.StatusBadRequest, errors.New("invalid remote address"))
		}

		instance, err := client.GetHackInstance(ctx, ip)
		if err != nil {
			_ = ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		ctx.JSON(200, instance)
	})
}
