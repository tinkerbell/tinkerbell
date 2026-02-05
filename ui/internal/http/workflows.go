package webhttp

import (
	"fmt"
	"strconv"

	"github.com/go-logr/logr"

	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	"sigs.k8s.io/yaml"
)

const (
	nameSingularWorkflow = "Workflow"
	namePluralWorkflow   = "Workflows"
)

// HandleWorkflowList handles the workflow list page route.
func HandleWorkflowList(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	client, err := GetKubeClientFromGinContext(c)
	if err != nil {
		log.Error(err, "Failed to get kube client")
		if HandleAuthError(c, err, log) {
			return
		}
		c.String(500, "Internal server error")
		return
	}
	namespaces := GetKubeNamespaces(ctx, c, client, log)
	selectedNamespace := GetSelectedNamespace(c, namespaces)

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 1
	}

	itemsPerPageStr := c.DefaultQuery("per_page", strconv.Itoa(DefaultItemsPerPage))
	itemsPerPage, _ := strconv.Atoi(itemsPerPageStr)
	itemsPerPage = ValidateItemsPerPage(itemsPerPage)

	var workflows []templates.Workflow

	workflowList, err := client.ListWorkflows(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list "+namePluralWorkflow, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, wf := range workflowList.Items {
			task := ""
			action := ""
			agent := ""
			if wf.Status.CurrentState != nil {
				task = wf.Status.CurrentState.TaskName
				action = wf.Status.CurrentState.ActionName
				agent = wf.Status.CurrentState.AgentID
			}
			webWf := templates.Workflow{
				Name:        wf.Name,
				Namespace:   wf.Namespace,
				TemplateRef: wf.Spec.TemplateRef,
				State:       string(wf.Status.State),
				Task:        task,
				Action:      action,
				Agent:       agent,
				CreatedAt:   wf.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			workflows = append(workflows, webWf)
		}
	}

	workflowPageData := GetPaginatedWorkflows(workflows, page, itemsPerPage)

	if IsHTMXRequest(c) {
		component := templates.WorkflowTableContent(workflowPageData)
		c.Header("Content-Type", "text/html")
		RenderComponent(c.Request.Context(), c.Writer, component, log)
		return
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.WorkflowPage(cfg, workflowPageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleWorkflowData handles the workflow data endpoint (HTMX partial).
func HandleWorkflowData(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	client, err := GetKubeClientFromGinContext(c)
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

	var workflows []templates.Workflow

	workflowList, err := client.ListWorkflows(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list "+namePluralWorkflow, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, wf := range workflowList.Items {
			task := ""
			action := ""
			agent := ""
			if wf.Status.CurrentState != nil {
				task = wf.Status.CurrentState.TaskName
				action = wf.Status.CurrentState.ActionName
				agent = wf.Status.CurrentState.AgentID
			}
			webWf := templates.Workflow{
				Name:        wf.Name,
				Namespace:   wf.Namespace,
				TemplateRef: wf.Spec.TemplateRef,
				State:       string(wf.Status.State),
				Task:        task,
				Action:      action,
				Agent:       agent,
				CreatedAt:   wf.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			workflows = append(workflows, webWf)
		}
	}

	workflowPageData := GetPaginatedWorkflows(workflows, page, itemsPerPage)

	component := templates.WorkflowTableContent(workflowPageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleWorkflowDetail handles the workflow detail page route.
func HandleWorkflowDetail(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	client, err := GetKubeClientFromGinContext(c)
	if err != nil {
		log.Error(err, "Failed to get kube client")
		if HandleAuthError(c, err, log) {
			return
		}
		c.String(500, "Internal server error")
		return
	}
	namespace := c.Param("namespace")
	name := c.Param("name")

	namespaces := GetKubeNamespaces(ctx, c, client, log)

	wf, err := client.GetWorkflow(ctx, namespace, name)
	if err != nil {
		log.V(1).Info("Failed to fetch "+nameSingularWorkflow, "namespace", namespace, "name", name, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
		cfg := templates.PageConfig{
			BaseURL:    GetBaseURL(c),
			Namespaces: namespaces,
		}
		component := templates.NotFoundPage(cfg, nameSingularWorkflow, name, namespace, "/workflows", namePluralWorkflow, fmt.Sprintf("This %s may have been deleted.", nameSingularWorkflow))
		c.Header("Content-Type", "text/html")
		c.Status(404)
		RenderComponent(ctx, c.Writer, component, log)
		return
	}

	yamlBytes, _ := yaml.Marshal(wf)
	specYAML, _ := yaml.Marshal(&wf.Spec)
	statusYAML, _ := yaml.Marshal(&wf.Status)

	task := ""
	action := ""
	agent := ""
	if wf.Status.CurrentState != nil {
		task = wf.Status.CurrentState.TaskName
		action = wf.Status.CurrentState.ActionName
		agent = wf.Status.CurrentState.AgentID
	}

	wfDetail := templates.WorkflowDetail{
		Name:              wf.Name,
		Namespace:         wf.Namespace,
		TemplateRef:       wf.Spec.TemplateRef,
		HardwareRef:       wf.Spec.HardwareRef,
		State:             string(wf.Status.State),
		Task:              task,
		Action:            action,
		Agent:             agent,
		TemplateRendering: string(wf.Status.TemplateRendering),
		CreatedAt:         wf.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
		Labels:            wf.Labels,
		Annotations:       wf.Annotations,
		SpecYAML:          string(specYAML),
		StatusYAML:        string(statusYAML),
		YAML:              string(yamlBytes),
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.WorkflowDetailPage(cfg, wfDetail)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}
