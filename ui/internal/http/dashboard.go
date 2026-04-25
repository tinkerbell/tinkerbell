package webhttp

import (
	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/crd"
	"github.com/tinkerbell/tinkerbell/ui/templates"
)

// HandleDashboard handles the landing page / CRD browser.
func HandleDashboard(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	client, _ := GetKubeClientFromGinContext(c)
	namespaces := GetKubeNamespaces(ctx, c, client, log)

	version := c.DefaultQuery("version", "v1alpha1")
	baseURL := GetBaseURL(c)
	dashboardData := GetDashboardDataForVersion(version)
	dashboardData.SelectedVersion = version
	dashboardData.AvailableVersions = crd.AvailableVersions
	dashboardData.BaseURL = baseURL

	cfg := templates.PageConfig{
		BaseURL:    baseURL,
		Namespaces: namespaces,
	}
	component := templates.DashboardPage(cfg, dashboardData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleDashboardData handles the HTMX partial endpoint for the CRD content area.
func HandleDashboardData(c *gin.Context, log logr.Logger) {
	version := c.DefaultQuery("version", "v1alpha1")
	dashboardData := GetDashboardDataForVersion(version)
	dashboardData.SelectedVersion = version
	dashboardData.AvailableVersions = crd.AvailableVersions
	dashboardData.BaseURL = GetBaseURL(c)

	component := templates.DashboardCRDContent(dashboardData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}
