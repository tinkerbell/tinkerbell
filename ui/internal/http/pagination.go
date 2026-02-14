package webhttp

import (
	"math"

	webtpl "github.com/tinkerbell/tinkerbell/ui/templates"
)

// GetPaginatedHardware creates paginated hardware data.
func GetPaginatedHardware(hardware []webtpl.Hardware, page, itemsPerPage int) webtpl.HardwarePageData {
	totalItems := len(hardware)
	totalPages := int(math.Ceil(float64(totalItems) / float64(itemsPerPage)))

	if totalItems == 0 {
		totalPages = 1
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * itemsPerPage
	endIndex := startIndex + itemsPerPage

	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > totalItems {
		endIndex = totalItems
	}
	if startIndex > totalItems {
		startIndex = totalItems
	}

	var paginatedHardware []webtpl.Hardware
	if startIndex < totalItems && startIndex >= 0 && endIndex >= startIndex {
		paginatedHardware = hardware[startIndex:endIndex]
	}

	startItem := 0
	endItem := 0
	if totalItems > 0 {
		startItem = startIndex + 1
		endItem = endIndex
	}

	return webtpl.HardwarePageData{
		Hardware: paginatedHardware,
		Pagination: webtpl.PaginationData{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: itemsPerPage,
			StartItem:    startItem,
			EndItem:      endItem,
			ResourcePath: "/hardware",
			TargetID:     "#hardware-content",
		},
	}
}

// GetPaginatedWorkflows creates paginated workflow data.
func GetPaginatedWorkflows(workflows []webtpl.Workflow, page, itemsPerPage int) webtpl.WorkflowPageData {
	totalItems := len(workflows)
	totalPages := int(math.Ceil(float64(totalItems) / float64(itemsPerPage)))

	if totalItems == 0 {
		totalPages = 1
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * itemsPerPage
	endIndex := startIndex + itemsPerPage

	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > totalItems {
		endIndex = totalItems
	}
	if startIndex > totalItems {
		startIndex = totalItems
	}

	var paginatedWorkflows []webtpl.Workflow
	if startIndex < totalItems && startIndex >= 0 && endIndex >= startIndex {
		paginatedWorkflows = workflows[startIndex:endIndex]
	}

	startItem := 0
	endItem := 0
	if totalItems > 0 {
		startItem = startIndex + 1
		endItem = endIndex
	}

	return webtpl.WorkflowPageData{
		Workflows: paginatedWorkflows,
		Pagination: webtpl.PaginationData{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: itemsPerPage,
			StartItem:    startItem,
			EndItem:      endItem,
			ResourcePath: "/workflows",
			TargetID:     "#workflow-content",
		},
	}
}

// GetPaginatedWorkflowRuleSets creates paginated workflowruleset data.
func GetPaginatedWorkflowRuleSets(rulesets []webtpl.WorkflowRuleSet, page, itemsPerPage int) webtpl.WorkflowRuleSetPageData {
	totalItems := len(rulesets)
	totalPages := int(math.Ceil(float64(totalItems) / float64(itemsPerPage)))

	if totalItems == 0 {
		totalPages = 1
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * itemsPerPage
	endIndex := startIndex + itemsPerPage

	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > totalItems {
		endIndex = totalItems
	}
	if startIndex > totalItems {
		startIndex = totalItems
	}

	var paginatedRuleSets []webtpl.WorkflowRuleSet
	if startIndex < totalItems && startIndex >= 0 && endIndex >= startIndex {
		paginatedRuleSets = rulesets[startIndex:endIndex]
	}

	startItem := 0
	endItem := 0
	if totalItems > 0 {
		startItem = startIndex + 1
		endItem = endIndex
	}

	return webtpl.WorkflowRuleSetPageData{
		RuleSets: paginatedRuleSets,
		Pagination: webtpl.PaginationData{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: itemsPerPage,
			StartItem:    startItem,
			EndItem:      endItem,
			ResourcePath: "/workflows/rulesets",
			TargetID:     "#workflowruleset-content",
		},
	}
}

// GetPaginatedTemplates creates paginated template data.
func GetPaginatedTemplates(templates []webtpl.Template, page, itemsPerPage int) webtpl.TemplatePageData {
	totalItems := len(templates)
	totalPages := int(math.Ceil(float64(totalItems) / float64(itemsPerPage)))

	if totalItems == 0 {
		totalPages = 1
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * itemsPerPage
	endIndex := startIndex + itemsPerPage

	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > totalItems {
		endIndex = totalItems
	}
	if startIndex > totalItems {
		startIndex = totalItems
	}

	var paginatedTemplates []webtpl.Template
	if startIndex < totalItems && startIndex >= 0 && endIndex >= startIndex {
		paginatedTemplates = templates[startIndex:endIndex]
	}

	startItem := 0
	endItem := 0
	if totalItems > 0 {
		startItem = startIndex + 1
		endItem = endIndex
	}

	return webtpl.TemplatePageData{
		Templates: paginatedTemplates,
		Pagination: webtpl.PaginationData{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: itemsPerPage,
			StartItem:    startItem,
			EndItem:      endItem,
			ResourcePath: "/templates",
			TargetID:     "#templates-content",
		},
	}
}

// GetPaginatedBMCMachines creates paginated BMC machine data.
func GetPaginatedBMCMachines(machines []webtpl.BMCMachine, page, itemsPerPage int) webtpl.BMCMachinePageData {
	totalItems := len(machines)
	totalPages := int(math.Ceil(float64(totalItems) / float64(itemsPerPage)))

	if totalItems == 0 {
		totalPages = 1
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * itemsPerPage
	endIndex := startIndex + itemsPerPage

	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > totalItems {
		endIndex = totalItems
	}
	if startIndex > totalItems {
		startIndex = totalItems
	}

	var paginatedMachines []webtpl.BMCMachine
	if startIndex < totalItems && startIndex >= 0 && endIndex >= startIndex {
		paginatedMachines = machines[startIndex:endIndex]
	}

	startItem := 0
	endItem := 0
	if totalItems > 0 {
		startItem = startIndex + 1
		endItem = endIndex
	}

	return webtpl.BMCMachinePageData{
		Machines: paginatedMachines,
		Pagination: webtpl.PaginationData{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: itemsPerPage,
			StartItem:    startItem,
			EndItem:      endItem,
			ResourcePath: "/bmc/machines",
			TargetID:     "#bmc-machines-content",
		},
	}
}

// GetPaginatedBMCJobs creates paginated BMC job data.
func GetPaginatedBMCJobs(jobs []webtpl.BMCJob, page, itemsPerPage int) webtpl.BMCJobPageData {
	totalItems := len(jobs)
	totalPages := int(math.Ceil(float64(totalItems) / float64(itemsPerPage)))

	if totalItems == 0 {
		totalPages = 1
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * itemsPerPage
	endIndex := startIndex + itemsPerPage

	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > totalItems {
		endIndex = totalItems
	}
	if startIndex > totalItems {
		startIndex = totalItems
	}

	var paginatedJobs []webtpl.BMCJob
	if startIndex < totalItems && startIndex >= 0 && endIndex >= startIndex {
		paginatedJobs = jobs[startIndex:endIndex]
	}

	startItem := 0
	endItem := 0
	if totalItems > 0 {
		startItem = startIndex + 1
		endItem = endIndex
	}

	return webtpl.BMCJobPageData{
		Jobs: paginatedJobs,
		Pagination: webtpl.PaginationData{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: itemsPerPage,
			StartItem:    startItem,
			EndItem:      endItem,
			ResourcePath: "/bmc/jobs",
			TargetID:     "#bmc-jobs-content",
		},
	}
}

// GetPaginatedBMCTasks creates paginated BMC task data.
func GetPaginatedBMCTasks(tasks []webtpl.BMCTask, page, itemsPerPage int) webtpl.BMCTaskPageData {
	totalItems := len(tasks)
	totalPages := int(math.Ceil(float64(totalItems) / float64(itemsPerPage)))

	if totalItems == 0 {
		totalPages = 1
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * itemsPerPage
	endIndex := startIndex + itemsPerPage

	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > totalItems {
		endIndex = totalItems
	}
	if startIndex > totalItems {
		startIndex = totalItems
	}

	var paginatedTasks []webtpl.BMCTask
	if startIndex < totalItems && startIndex >= 0 && endIndex >= startIndex {
		paginatedTasks = tasks[startIndex:endIndex]
	}

	startItem := 0
	endItem := 0
	if totalItems > 0 {
		startItem = startIndex + 1
		endItem = endIndex
	}

	return webtpl.BMCTaskPageData{
		Tasks: paginatedTasks,
		Pagination: webtpl.PaginationData{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: itemsPerPage,
			StartItem:    startItem,
			EndItem:      endItem,
			ResourcePath: "/bmc/tasks",
			TargetID:     "#bmc-tasks-content",
		},
	}
}
