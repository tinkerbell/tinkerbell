package webhttp

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	"sigs.k8s.io/yaml"
)

const (
	nameSingularWorkflowRuleSet = "Ruleset"
	namePluralWorkflowRuleSet   = "Rulesets"
)

// HandleWorkflowRuleSetList handles the workflowruleset list page route.
func HandleWorkflowRuleSetList(c *gin.Context, log logr.Logger) {
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

	var rulesets []templates.WorkflowRuleSet

	rulesetList, err := client.ListWorkflowRuleSets(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list "+namePluralWorkflowRuleSet, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, rs := range rulesetList.Items {
			rules := "N/A"
			if len(rs.Spec.Rules) > 0 {
				rulesBytes, _ := yaml.Marshal(rs.Spec.Rules)
				rules = string(rulesBytes)
			}

			templateRef := "N/A"
			if rs.Spec.Workflow.Template.Ref != "" {
				templateRef = rs.Spec.Workflow.Template.Ref
			}

			webRS := templates.WorkflowRuleSet{
				Name:        rs.Name,
				Namespace:   rs.Namespace,
				Rules:       rules,
				TemplateRef: templateRef,
				CreatedAt:   rs.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			rulesets = append(rulesets, webRS)
		}
	}

	rulesetPageData := GetPaginatedWorkflowRuleSets(rulesets, page, itemsPerPage)

	if IsHTMXRequest(c) {
		component := templates.WorkflowRuleSetTableContent(rulesetPageData)
		c.Header("Content-Type", "text/html")
		RenderComponent(c.Request.Context(), c.Writer, component, log)
		return
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.WorkflowRuleSetPage(cfg, rulesetPageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleWorkflowRuleSetData handles the workflowruleset data endpoint (HTMX partial).
func HandleWorkflowRuleSetData(c *gin.Context, log logr.Logger) {
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

	var rulesets []templates.WorkflowRuleSet

	rulesetList, err := client.ListWorkflowRuleSets(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list "+namePluralWorkflowRuleSet, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, rs := range rulesetList.Items {
			rules := "N/A"
			if len(rs.Spec.Rules) > 0 {
				rulesBytes, _ := yaml.Marshal(rs.Spec.Rules)
				rules = string(rulesBytes)
			}

			templateRef := "N/A"
			if rs.Spec.Workflow.Template.Ref != "" {
				templateRef = rs.Spec.Workflow.Template.Ref
			}

			webRS := templates.WorkflowRuleSet{
				Name:        rs.Name,
				Namespace:   rs.Namespace,
				Rules:       rules,
				TemplateRef: templateRef,
				CreatedAt:   rs.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			rulesets = append(rulesets, webRS)
		}
	}

	rulesetPageData := GetPaginatedWorkflowRuleSets(rulesets, page, itemsPerPage)

	component := templates.WorkflowRuleSetTableContent(rulesetPageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleWorkflowRuleSetDetail handles the workflowruleset detail page route.
func HandleWorkflowRuleSetDetail(c *gin.Context, log logr.Logger) {
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

	rs, err := client.GetWorkflowRuleSet(ctx, namespace, name)
	if err != nil {
		log.V(1).Info("Failed to fetch "+nameSingularWorkflowRuleSet, "namespace", namespace, "name", name, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
		cfg := templates.PageConfig{
			BaseURL:    GetBaseURL(c),
			Namespaces: namespaces,
		}
		component := templates.NotFoundPage(cfg, nameSingularWorkflowRuleSet, name, namespace, "/workflows/rulesets", namePluralWorkflowRuleSet, fmt.Sprintf("This %s may have been deleted.", nameSingularWorkflowRuleSet))
		c.Header("Content-Type", "text/html")
		RenderComponent(c.Request.Context(), c.Writer, component, log)
		return
	}

	yamlData, err := yaml.Marshal(rs)
	if err != nil {
		log.Error(err, "Failed to marshal workflowruleset to YAML")
		yamlData = []byte("Error: Could not marshal data to YAML")
	}

	// Extract workflow disabled status
	workflowDisabled := false
	if rs.Spec.Workflow.Disabled != nil {
		workflowDisabled = *rs.Spec.Workflow.Disabled
	}

	detailData := templates.WorkflowRuleSetDetail{
		Name:              rs.Name,
		Namespace:         rs.Namespace,
		YAMLData:          string(yamlData),
		CreatedAt:         rs.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
		Labels:            rs.Labels,
		Annotations:       rs.Annotations,
		Rules:             rs.Spec.Rules,
		TemplateRef:       rs.Spec.Workflow.Template.Ref,
		WorkflowNamespace: rs.Spec.Workflow.Namespace,
		WorkflowDisabled:  workflowDisabled,
		AddAttributes:     rs.Spec.Workflow.AddAttributes,
		AgentValue:        rs.Spec.Workflow.Template.AgentValue,
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.WorkflowRuleSetDetailPage(cfg, detailData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}
