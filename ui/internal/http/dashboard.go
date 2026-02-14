package webhttp

import (
	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/ui/templates"
)

// HandleDashboard handles the landing page / CRD browser.
func HandleDashboard(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	client, _ := GetKubeClientFromGinContext(c)
	namespaces := GetKubeNamespaces(ctx, c, client, log)

	dashboardData := GetDashboardData()

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.DashboardPage(cfg, dashboardData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}
