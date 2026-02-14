package webhttp

import (
	"strings"

	"github.com/go-logr/logr"

	"github.com/gin-gonic/gin"
)

const (
	// MaxSearchResults is the maximum total number of search results to return.
	MaxSearchResults = 20
	// MaxSearchResultsPerType is the maximum number of results per resource type.
	MaxSearchResultsPerType = 5
	// MinSearchQueryLength is the minimum query length required to perform a search.
	// This prevents overly broad searches that would fetch all resources.
	MinSearchQueryLength = 3
)

// SearchResult represents a single search result.
type SearchResult struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	TypeLabel string `json:"typeLabel"`
	URL       string `json:"url"`
	Icon      string `json:"icon"`
}

// SearchResponse is the response for the global search endpoint.
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Query   string         `json:"query"`
	Message string         `json:"message,omitempty"`
}

// HandleGlobalSearch handles the global search API endpoint.
func HandleGlobalSearch(c *gin.Context, log logr.Logger) {
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	namespace := c.Query("namespace")

	kubeClient, err := GetKubeClientFromGinContext(c)
	if err != nil {
		log.Error(err, "Failed to get kube client")
		if HandleAuthError(c, err, log) {
			return
		}
		c.JSON(500, gin.H{"error": "Internal server error"})
		return
	}

	if query == "" {
		c.JSON(200, SearchResponse{Results: []SearchResult{}, Query: query})
		return
	}

	// Require minimum query length to prevent overly broad searches
	if len(query) < MinSearchQueryLength {
		c.JSON(200, SearchResponse{Results: []SearchResult{}, Query: query, Message: "Query must be at least 3 characters"})
		return
	}

	var results []SearchResult

	results = append(results, searchHardware(c, kubeClient, query, namespace, log)...)
	results = append(results, searchWorkflows(c, kubeClient, query, namespace, log)...)
	results = append(results, searchTemplates(c, kubeClient, query, namespace, log)...)
	results = append(results, searchBMCMachines(c, kubeClient, query, namespace, log)...)
	results = append(results, searchBMCJobs(c, kubeClient, query, namespace, log)...)
	results = append(results, searchBMCTasks(c, kubeClient, query, namespace, log)...)

	// Limit total results to prevent overwhelming the user (QUAL-1)
	if len(results) > MaxSearchResults {
		results = results[:MaxSearchResults]
	}

	c.JSON(200, SearchResponse{Results: results, Query: query})
}

func searchHardware(c *gin.Context, kubeClient *KubeClient, query, namespace string, log logr.Logger) []SearchResult {
	var results []SearchResult
	ctx := c.Request.Context()

	hardwareList, err := kubeClient.ListHardware(ctx, namespace)
	if err != nil {
		// Use consistent error logging format (QUAL-2)
		log.Error(err, "Failed to list hardware for search",
			"namespace", namespace,
			"query", query,
		)
		if HandleAuthError(c, err, log) {
			return results
		}
		return results
	}

	// Use early return pattern to reduce nesting (IDIOM-9)
	for _, hw := range hardwareList.Items {
		name := strings.ToLower(hw.Name)
		ns := strings.ToLower(hw.Namespace)

		if !strings.Contains(name, query) && !strings.Contains(ns, query) {
			continue
		}

		results = append(results, SearchResult{
			Name:      hw.Name,
			Namespace: hw.Namespace,
			Type:      "hardware",
			TypeLabel: "Hardware",
			URL:       "/hardware/" + hw.Namespace + "/" + hw.Name,
			Icon:      "hardware",
		})

		// Limit results per type (QUAL-1)
		if len(results) >= MaxSearchResultsPerType {
			break
		}
	}

	return results
}

func searchWorkflows(c *gin.Context, kubeClient *KubeClient, query, namespace string, log logr.Logger) []SearchResult {
	var results []SearchResult
	ctx := c.Request.Context()

	workflowList, err := kubeClient.ListWorkflows(ctx, namespace)
	if err != nil {
		log.Error(err, "Failed to list workflows for search",
			"namespace", namespace,
			"query", query,
		)
		if HandleAuthError(c, err, log) {
			return results
		}
		return results
	}

	for _, wf := range workflowList.Items {
		name := strings.ToLower(wf.Name)
		ns := strings.ToLower(wf.Namespace)

		if !strings.Contains(name, query) && !strings.Contains(ns, query) {
			continue
		}

		results = append(results, SearchResult{
			Name:      wf.Name,
			Namespace: wf.Namespace,
			Type:      "workflow",
			TypeLabel: "Workflow",
			URL:       "/workflows/" + wf.Namespace + "/" + wf.Name,
			Icon:      "workflow",
		})

		if len(results) >= MaxSearchResultsPerType {
			break
		}
	}

	return results
}

func searchTemplates(c *gin.Context, kubeClient *KubeClient, query, namespace string, log logr.Logger) []SearchResult {
	var results []SearchResult
	ctx := c.Request.Context()

	templateList, err := kubeClient.ListTemplates(ctx, namespace)
	if err != nil {
		log.Error(err, "Failed to list templates for search",
			"namespace", namespace,
			"query", query,
		)
		if HandleAuthError(c, err, log) {
			return results
		}
		return results
	}

	for _, tmpl := range templateList.Items {
		name := strings.ToLower(tmpl.Name)
		ns := strings.ToLower(tmpl.Namespace)

		if !strings.Contains(name, query) && !strings.Contains(ns, query) {
			continue
		}

		results = append(results, SearchResult{
			Name:      tmpl.Name,
			Namespace: tmpl.Namespace,
			Type:      "template",
			TypeLabel: "Template",
			URL:       "/templates/" + tmpl.Namespace + "/" + tmpl.Name,
			Icon:      "template",
		})

		if len(results) >= MaxSearchResultsPerType {
			break
		}
	}

	return results
}

func searchBMCMachines(c *gin.Context, kubeClient *KubeClient, query, namespace string, log logr.Logger) []SearchResult {
	var results []SearchResult
	ctx := c.Request.Context()

	machineList, err := kubeClient.ListBMCMachines(ctx, namespace)
	if err != nil {
		log.Error(err, "Failed to list BMC machines for search",
			"namespace", namespace,
			"query", query,
		)
		if HandleAuthError(c, err, log) {
			return results
		}
		return results
	}

	for _, machine := range machineList.Items {
		name := strings.ToLower(machine.Name)
		ns := strings.ToLower(machine.Namespace)

		if !strings.Contains(name, query) && !strings.Contains(ns, query) {
			continue
		}

		results = append(results, SearchResult{
			Name:      machine.Name,
			Namespace: machine.Namespace,
			Type:      "bmc-machine",
			TypeLabel: "BMC Machine",
			URL:       "/bmc/machines/" + machine.Namespace + "/" + machine.Name,
			Icon:      "bmc",
		})

		if len(results) >= MaxSearchResultsPerType {
			break
		}
	}

	return results
}

func searchBMCJobs(c *gin.Context, kubeClient *KubeClient, query, namespace string, log logr.Logger) []SearchResult {
	var results []SearchResult
	ctx := c.Request.Context()

	jobList, err := kubeClient.ListBMCJobs(ctx, namespace)
	if err != nil {
		log.Error(err, "Failed to list BMC jobs for search",
			"namespace", namespace,
			"query", query,
		)
		if HandleAuthError(c, err, log) {
			return results
		}
		return results
	}

	for _, job := range jobList.Items {
		name := strings.ToLower(job.Name)
		ns := strings.ToLower(job.Namespace)

		if !strings.Contains(name, query) && !strings.Contains(ns, query) {
			continue
		}

		results = append(results, SearchResult{
			Name:      job.Name,
			Namespace: job.Namespace,
			Type:      "bmc-job",
			TypeLabel: "BMC Job",
			URL:       "/bmc/jobs/" + job.Namespace + "/" + job.Name,
			Icon:      "bmc",
		})

		if len(results) >= MaxSearchResultsPerType {
			break
		}
	}

	return results
}

func searchBMCTasks(c *gin.Context, kubeClient *KubeClient, query, namespace string, log logr.Logger) []SearchResult {
	var results []SearchResult
	ctx := c.Request.Context()

	taskList, err := kubeClient.ListBMCTasks(ctx, namespace)
	if err != nil {
		log.Error(err, "Failed to list BMC tasks for search",
			"namespace", namespace,
			"query", query,
		)
		if HandleAuthError(c, err, log) {
			return results
		}
		return results
	}

	for _, task := range taskList.Items {
		name := strings.ToLower(task.Name)
		ns := strings.ToLower(task.Namespace)

		if !strings.Contains(name, query) && !strings.Contains(ns, query) {
			continue
		}

		results = append(results, SearchResult{
			Name:      task.Name,
			Namespace: task.Namespace,
			Type:      "bmc-task",
			TypeLabel: "BMC Task",
			URL:       "/bmc/tasks/" + task.Namespace + "/" + task.Name,
			Icon:      "bmc",
		})

		if len(results) >= MaxSearchResultsPerType {
			break
		}
	}

	return results
}
