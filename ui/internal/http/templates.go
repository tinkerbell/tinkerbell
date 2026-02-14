package webhttp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	webtpl "github.com/tinkerbell/tinkerbell/ui/templates"
	"sigs.k8s.io/yaml"
)

const (
	nameSingularTemplate = "Template"
	namePluralTemplate   = "Templates"
)

// HandleTemplateList handles the template list page route.
func HandleTemplateList(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	kubeClient, err := GetKubeClientFromGinContext(c)
	if err != nil {
		log.Error(err, "Failed to get kube client")
		if HandleAuthError(c, err, log) {
			return
		}
		c.String(500, "Internal server error")
		return
	}
	namespaces := GetKubeNamespaces(ctx, c, kubeClient, log)
	selectedNamespace := GetSelectedNamespace(c, namespaces)

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 1
	}

	itemsPerPageStr := c.DefaultQuery("per_page", strconv.Itoa(DefaultItemsPerPage))
	itemsPerPage, _ := strconv.Atoi(itemsPerPageStr)
	itemsPerPage = ValidateItemsPerPage(itemsPerPage)

	var templates []webtpl.Template

	templateList, err := kubeClient.ListTemplates(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list "+namePluralTemplate, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, tpl := range templateList.Items {
			data := ""
			if tpl.Spec.Data != nil {
				data = *tpl.Spec.Data
			}
			webTpl := webtpl.Template{
				Name:      tpl.Name,
				Namespace: tpl.Namespace,
				State:     string(tpl.Status.State),
				Data:      data,
				CreatedAt: tpl.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			templates = append(templates, webTpl)
		}
	}

	templatePageData := GetPaginatedTemplates(templates, page, itemsPerPage)

	if IsHTMXRequest(c) {
		component := webtpl.TemplateTableContent(templatePageData)
		c.Header("Content-Type", "text/html")
		RenderComponent(c.Request.Context(), c.Writer, component, log)
		return
	}

	cfg := webtpl.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := webtpl.TemplatePage(cfg, templatePageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleTemplateData handles the template data endpoint (HTMX partial).
func HandleTemplateData(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	kubeClient, err := GetKubeClientFromGinContext(c)
	if err != nil {
		log.Error(err, "Failed to get kube client")
		if HandleAuthError(c, err, log) {
			return
		}
		c.String(500, "Internal server error")
		return
	}
	selectedNamespace := c.Query("namespace")

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 1
	}

	itemsPerPageStr := c.DefaultQuery("per_page", strconv.Itoa(DefaultItemsPerPage))
	itemsPerPage, _ := strconv.Atoi(itemsPerPageStr)
	itemsPerPage = ValidateItemsPerPage(itemsPerPage)

	var templates []webtpl.Template

	templateList, err := kubeClient.ListTemplates(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list "+namePluralTemplate, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, tpl := range templateList.Items {
			data := ""
			if tpl.Spec.Data != nil {
				data = *tpl.Spec.Data
			}
			webTpl := webtpl.Template{
				Name:      tpl.Name,
				Namespace: tpl.Namespace,
				State:     string(tpl.Status.State),
				Data:      data,
				CreatedAt: tpl.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			templates = append(templates, webTpl)
		}
	}

	templatePageData := GetPaginatedTemplates(templates, page, itemsPerPage)

	component := webtpl.TemplateTableContent(templatePageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleTemplateDetail handles the template detail page route.
func HandleTemplateDetail(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	kubeClient, err := GetKubeClientFromGinContext(c)
	if err != nil {
		log.Error(err, "Failed to get kube client")
		if HandleAuthError(c, err, log) {
			return
		}
		c.String(500, "Internal server error")
		return
	}
	namespaces := GetKubeNamespaces(ctx, c, kubeClient, log)

	tpl, err := kubeClient.GetTemplate(ctx, namespace, name)
	if err != nil {
		log.V(1).Info("Failed to fetch "+nameSingularTemplate, "namespace", namespace, "name", name, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
		cfg := webtpl.PageConfig{
			BaseURL:    GetBaseURL(c),
			Namespaces: namespaces,
		}
		component := webtpl.NotFoundPage(cfg, nameSingularTemplate, name, namespace, "/templates", namePluralTemplate, fmt.Sprintf("This %s may have been deleted.", nameSingularTemplate))
		c.Header("Content-Type", "text/html")
		c.Status(404)
		RenderComponent(ctx, c.Writer, component, log)
		return
	}

	yamlBytes, _ := yaml.Marshal(tpl)
	specYAML, _ := yaml.Marshal(&tpl.Spec)
	statusYAML, _ := yaml.Marshal(&tpl.Status)

	data := ""
	if tpl.Spec.Data != nil {
		data = *tpl.Spec.Data
		data = strings.ReplaceAll(data, "\\n", "\n")
		data = strings.ReplaceAll(data, "\\t", "\t")
	}

	tplDetail := webtpl.TemplateDetail{
		Name:        tpl.Name,
		Namespace:   tpl.Namespace,
		State:       string(tpl.Status.State),
		Data:        data,
		CreatedAt:   tpl.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
		Labels:      tpl.Labels,
		Annotations: tpl.Annotations,
		SpecYAML:    string(specYAML),
		StatusYAML:  string(statusYAML),
		YAML:        string(yamlBytes),
	}

	cfg := webtpl.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := webtpl.TemplateDetailPage(cfg, tplDetail)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}
