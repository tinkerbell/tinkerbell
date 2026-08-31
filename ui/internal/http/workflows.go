package webhttp

import (
	"context"
	"fmt"
	"strconv"

	"github.com/a-h/templ"
	"github.com/go-logr/logr"

	"github.com/gin-gonic/gin"
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/yaml"
)

const (
	nameSingularWorkflow = "Workflow"
	namePluralWorkflow   = "Workflows"
)

// isWorkflowDisabled reports whether spec.disabled is set and true.
func isWorkflowDisabled(wf *tinkv1alpha1.Workflow) bool {
	return wf.Spec.Disabled != nil && *wf.Spec.Disabled
}

// toWorkflowRow converts a Workflow resource into its list-view representation.
// CanUpdate is left false; call setCanUpdatePermissions afterwards to fill it
// in for disabled rows.
func toWorkflowRow(wf tinkv1alpha1.Workflow) templates.Workflow {
	task := ""
	action := ""
	agent := ""
	if wf.Status.CurrentState != nil {
		task = wf.Status.CurrentState.TaskName
		action = wf.Status.CurrentState.ActionName
		agent = wf.Status.CurrentState.AgentID
	}
	return templates.Workflow{
		Name:        wf.Name,
		Namespace:   wf.Namespace,
		TemplateRef: wf.Spec.TemplateRef,
		State:       string(wf.Status.State),
		Disabled:    isWorkflowDisabled(&wf),
		Task:        task,
		Action:      action,
		Agent:       agent,
		CreatedAt:   wf.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
	}
}

// setCanUpdatePermissions fills in CanUpdate for each disabled row in rows,
// checking patch permission on workflows.tinkerbell.org once per distinct
// namespace encountered (not once per row, and not at all if nothing in the
// list is disabled) since a workflow list can span multiple namespaces with
// different RBAC grants.
func setCanUpdatePermissions(ctx context.Context, client *KubeClient, log logr.Logger, rows []templates.Workflow) {
	cache := map[string]bool{}
	for i := range rows {
		if !rows[i].Disabled {
			continue
		}
		ns := rows[i].Namespace
		can, ok := cache[ns]
		if !ok {
			can = canPatchWorkflows(ctx, client, log, ns)
			cache[ns] = can
		}
		rows[i].CanUpdate = can
	}
}

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
			workflows = append(workflows, toWorkflowRow(wf))
		}
		setCanUpdatePermissions(ctx, client, log, workflows)
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
			workflows = append(workflows, toWorkflowRow(wf))
		}
		setCanUpdatePermissions(ctx, client, log, workflows)
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
		Disabled:          isWorkflowDisabled(wf),
		CanUpdate:         canPatchWorkflows(ctx, client, log, namespace),
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

// HandleWorkflowEnable handles enabling a disabled workflow (setting spec.disabled
// to false) and returns the updated control fragment for an htmx swap. There is
// deliberately no way to disable a workflow through this endpoint: see WorkflowDisabledControl.
func HandleWorkflowEnable(c *gin.Context, log logr.Logger) {
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

	wf, err := client.EnableWorkflow(ctx, namespace, name)
	if err != nil {
		if HandleAuthError(c, err, log) {
			return
		}
		if apierrors.IsNotFound(err) {
			c.String(404, "Workflow not found")
			return
		}
		if apierrors.IsForbidden(err) {
			log.V(1).Info("Permission denied enabling "+nameSingularWorkflow, "namespace", namespace, "name", name, "error", err)
			c.String(403, "You don't have permission to enable this Workflow")
			return
		}
		log.Error(err, "Failed to update "+nameSingularWorkflow, "namespace", namespace, "name", name)
		c.String(500, "Failed to update workflow")
		return
	}

	canUpdate := canPatchWorkflows(ctx, client, log, namespace)

	var component templ.Component
	if c.PostForm("render") == "row" {
		row := toWorkflowRow(*wf)
		row.CanUpdate = canUpdate
		component = templates.WorkflowTableRow(row)
	} else {
		component = templates.WorkflowDisabledControl(templates.WorkflowDetail{
			Name:      wf.Name,
			Namespace: wf.Namespace,
			Disabled:  false,
			CanUpdate: canUpdate,
		})
	}
	c.Header("Content-Type", "text/html")
	RenderComponent(ctx, c.Writer, component, log)
}
