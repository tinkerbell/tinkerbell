package healthcheck

import "github.com/gin-gonic/gin"

// Configure configures router with a /healthz endpoint using a handler created with NewHandler.
func Configure(router gin.IRouter, client Client) {
	router.GET("/healthz", NewHandler(client))
}
