package webhttp

import (
	"testing"

	webtpl "github.com/tinkerbell/tinkerbell/ui/templates"
)

func TestGetPaginatedHardware(t *testing.T) {
	tests := []struct {
		name         string
		hardware     []webtpl.Hardware
		page         int
		itemsPerPage int
		wantPage     int
		wantTotal    int
		wantStart    int
		wantEnd      int
		wantCount    int
	}{
		{
			name:         "empty list",
			hardware:     []webtpl.Hardware{},
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    1,
			wantStart:    0,
			wantEnd:      0,
			wantCount:    0,
		},
		{
			name:         "first page of many",
			hardware:     makeHardwareList(25),
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    3,
			wantStart:    1,
			wantEnd:      10,
			wantCount:    10,
		},
		{
			name:         "middle page",
			hardware:     makeHardwareList(25),
			page:         2,
			itemsPerPage: 10,
			wantPage:     2,
			wantTotal:    3,
			wantStart:    11,
			wantEnd:      20,
			wantCount:    10,
		},
		{
			name:         "last partial page",
			hardware:     makeHardwareList(25),
			page:         3,
			itemsPerPage: 10,
			wantPage:     3,
			wantTotal:    3,
			wantStart:    21,
			wantEnd:      25,
			wantCount:    5,
		},
		{
			name:         "page beyond total clamped to last",
			hardware:     makeHardwareList(25),
			page:         10,
			itemsPerPage: 10,
			wantPage:     3,
			wantTotal:    3,
			wantStart:    21,
			wantEnd:      25,
			wantCount:    5,
		},
		{
			name:         "page zero clamped to one",
			hardware:     makeHardwareList(25),
			page:         0,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    3,
			wantStart:    1,
			wantEnd:      10,
			wantCount:    10,
		},
		{
			name:         "negative page clamped to one",
			hardware:     makeHardwareList(25),
			page:         -5,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    3,
			wantStart:    1,
			wantEnd:      10,
			wantCount:    10,
		},
		{
			name:         "exact page boundary",
			hardware:     makeHardwareList(20),
			page:         2,
			itemsPerPage: 10,
			wantPage:     2,
			wantTotal:    2,
			wantStart:    11,
			wantEnd:      20,
			wantCount:    10,
		},
		{
			name:         "single item",
			hardware:     makeHardwareList(1),
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    1,
			wantStart:    1,
			wantEnd:      1,
			wantCount:    1,
		},
		{
			name:         "items per page larger than total",
			hardware:     makeHardwareList(5),
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    1,
			wantStart:    1,
			wantEnd:      5,
			wantCount:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPaginatedHardware(tt.hardware, tt.page, tt.itemsPerPage)

			if result.Pagination.CurrentPage != tt.wantPage {
				t.Errorf("CurrentPage = %d, want %d", result.Pagination.CurrentPage, tt.wantPage)
			}
			if result.Pagination.TotalPages != tt.wantTotal {
				t.Errorf("TotalPages = %d, want %d", result.Pagination.TotalPages, tt.wantTotal)
			}
			if result.Pagination.StartItem != tt.wantStart {
				t.Errorf("StartItem = %d, want %d", result.Pagination.StartItem, tt.wantStart)
			}
			if result.Pagination.EndItem != tt.wantEnd {
				t.Errorf("EndItem = %d, want %d", result.Pagination.EndItem, tt.wantEnd)
			}
			if len(result.Hardware) != tt.wantCount {
				t.Errorf("len(Hardware) = %d, want %d", len(result.Hardware), tt.wantCount)
			}
			if result.Pagination.ResourcePath != "/hardware" {
				t.Errorf("ResourcePath = %s, want /hardware", result.Pagination.ResourcePath)
			}
			if result.Pagination.TargetID != "#hardware-content" {
				t.Errorf("TargetID = %s, want #hardware-content", result.Pagination.TargetID)
			}
		})
	}
}

func TestGetPaginatedWorkflows(t *testing.T) {
	tests := []struct {
		name         string
		workflows    []webtpl.Workflow
		page         int
		itemsPerPage int
		wantPage     int
		wantTotal    int
		wantCount    int
	}{
		{
			name:         "empty list",
			workflows:    []webtpl.Workflow{},
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    1,
			wantCount:    0,
		},
		{
			name:         "first page",
			workflows:    makeWorkflowList(15),
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    2,
			wantCount:    10,
		},
		{
			name:         "second page partial",
			workflows:    makeWorkflowList(15),
			page:         2,
			itemsPerPage: 10,
			wantPage:     2,
			wantTotal:    2,
			wantCount:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPaginatedWorkflows(tt.workflows, tt.page, tt.itemsPerPage)

			if result.Pagination.CurrentPage != tt.wantPage {
				t.Errorf("CurrentPage = %d, want %d", result.Pagination.CurrentPage, tt.wantPage)
			}
			if result.Pagination.TotalPages != tt.wantTotal {
				t.Errorf("TotalPages = %d, want %d", result.Pagination.TotalPages, tt.wantTotal)
			}
			if len(result.Workflows) != tt.wantCount {
				t.Errorf("len(Workflows) = %d, want %d", len(result.Workflows), tt.wantCount)
			}
			if result.Pagination.ResourcePath != "/workflows" {
				t.Errorf("ResourcePath = %s, want /workflows", result.Pagination.ResourcePath)
			}
		})
	}
}

func TestGetPaginatedTemplates(t *testing.T) {
	tests := []struct {
		name         string
		templates    []webtpl.Template
		page         int
		itemsPerPage int
		wantPage     int
		wantTotal    int
		wantCount    int
	}{
		{
			name:         "empty list",
			templates:    []webtpl.Template{},
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    1,
			wantCount:    0,
		},
		{
			name:         "single page",
			templates:    makeTemplateList(5),
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    1,
			wantCount:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPaginatedTemplates(tt.templates, tt.page, tt.itemsPerPage)

			if result.Pagination.CurrentPage != tt.wantPage {
				t.Errorf("CurrentPage = %d, want %d", result.Pagination.CurrentPage, tt.wantPage)
			}
			if result.Pagination.TotalPages != tt.wantTotal {
				t.Errorf("TotalPages = %d, want %d", result.Pagination.TotalPages, tt.wantTotal)
			}
			if len(result.Templates) != tt.wantCount {
				t.Errorf("len(Templates) = %d, want %d", len(result.Templates), tt.wantCount)
			}
			if result.Pagination.ResourcePath != "/templates" {
				t.Errorf("ResourcePath = %s, want /templates", result.Pagination.ResourcePath)
			}
		})
	}
}

func TestGetPaginatedBMCMachines(t *testing.T) {
	tests := []struct {
		name         string
		machines     []webtpl.BMCMachine
		page         int
		itemsPerPage int
		wantPage     int
		wantTotal    int
		wantCount    int
	}{
		{
			name:         "empty list",
			machines:     []webtpl.BMCMachine{},
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    1,
			wantCount:    0,
		},
		{
			name:         "multiple pages",
			machines:     makeBMCMachineList(22),
			page:         2,
			itemsPerPage: 10,
			wantPage:     2,
			wantTotal:    3,
			wantCount:    10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPaginatedBMCMachines(tt.machines, tt.page, tt.itemsPerPage)

			if result.Pagination.CurrentPage != tt.wantPage {
				t.Errorf("CurrentPage = %d, want %d", result.Pagination.CurrentPage, tt.wantPage)
			}
			if result.Pagination.TotalPages != tt.wantTotal {
				t.Errorf("TotalPages = %d, want %d", result.Pagination.TotalPages, tt.wantTotal)
			}
			if len(result.Machines) != tt.wantCount {
				t.Errorf("len(Machines) = %d, want %d", len(result.Machines), tt.wantCount)
			}
			if result.Pagination.ResourcePath != "/bmc/machines" {
				t.Errorf("ResourcePath = %s, want /bmc/machines", result.Pagination.ResourcePath)
			}
		})
	}
}

func TestGetPaginatedBMCJobs(t *testing.T) {
	tests := []struct {
		name         string
		jobs         []webtpl.BMCJob
		page         int
		itemsPerPage int
		wantPage     int
		wantTotal    int
		wantCount    int
	}{
		{
			name:         "empty list",
			jobs:         []webtpl.BMCJob{},
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    1,
			wantCount:    0,
		},
		{
			name:         "first page",
			jobs:         makeBMCJobList(12),
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    2,
			wantCount:    10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPaginatedBMCJobs(tt.jobs, tt.page, tt.itemsPerPage)

			if result.Pagination.CurrentPage != tt.wantPage {
				t.Errorf("CurrentPage = %d, want %d", result.Pagination.CurrentPage, tt.wantPage)
			}
			if result.Pagination.TotalPages != tt.wantTotal {
				t.Errorf("TotalPages = %d, want %d", result.Pagination.TotalPages, tt.wantTotal)
			}
			if len(result.Jobs) != tt.wantCount {
				t.Errorf("len(Jobs) = %d, want %d", len(result.Jobs), tt.wantCount)
			}
			if result.Pagination.ResourcePath != "/bmc/jobs" {
				t.Errorf("ResourcePath = %s, want /bmc/jobs", result.Pagination.ResourcePath)
			}
		})
	}
}

func TestGetPaginatedBMCTasks(t *testing.T) {
	tests := []struct {
		name         string
		tasks        []webtpl.BMCTask
		page         int
		itemsPerPage int
		wantPage     int
		wantTotal    int
		wantCount    int
	}{
		{
			name:         "empty list",
			tasks:        []webtpl.BMCTask{},
			page:         1,
			itemsPerPage: 10,
			wantPage:     1,
			wantTotal:    1,
			wantCount:    0,
		},
		{
			name:         "last page",
			tasks:        makeBMCTaskList(18),
			page:         2,
			itemsPerPage: 10,
			wantPage:     2,
			wantTotal:    2,
			wantCount:    8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPaginatedBMCTasks(tt.tasks, tt.page, tt.itemsPerPage)

			if result.Pagination.CurrentPage != tt.wantPage {
				t.Errorf("CurrentPage = %d, want %d", result.Pagination.CurrentPage, tt.wantPage)
			}
			if result.Pagination.TotalPages != tt.wantTotal {
				t.Errorf("TotalPages = %d, want %d", result.Pagination.TotalPages, tt.wantTotal)
			}
			if len(result.Tasks) != tt.wantCount {
				t.Errorf("len(Tasks) = %d, want %d", len(result.Tasks), tt.wantCount)
			}
			if result.Pagination.ResourcePath != "/bmc/tasks" {
				t.Errorf("ResourcePath = %s, want /bmc/tasks", result.Pagination.ResourcePath)
			}
		})
	}
}

// Helper functions to create test data.

func makeHardwareList(count int) []webtpl.Hardware {
	hardware := make([]webtpl.Hardware, count)
	for i := range count {
		hardware[i] = webtpl.Hardware{
			Name:      "hw-" + string(rune('a'+i%26)),
			Namespace: "default",
		}
	}
	return hardware
}

func makeWorkflowList(count int) []webtpl.Workflow {
	workflows := make([]webtpl.Workflow, count)
	for i := range count {
		workflows[i] = webtpl.Workflow{
			Name:      "wf-" + string(rune('a'+i%26)),
			Namespace: "default",
		}
	}
	return workflows
}

func makeTemplateList(count int) []webtpl.Template {
	templates := make([]webtpl.Template, count)
	for i := range count {
		templates[i] = webtpl.Template{
			Name:      "tmpl-" + string(rune('a'+i%26)),
			Namespace: "default",
		}
	}
	return templates
}

func makeBMCMachineList(count int) []webtpl.BMCMachine {
	machines := make([]webtpl.BMCMachine, count)
	for i := range count {
		machines[i] = webtpl.BMCMachine{
			Name:      "machine-" + string(rune('a'+i%26)),
			Namespace: "default",
		}
	}
	return machines
}

func makeBMCJobList(count int) []webtpl.BMCJob {
	jobs := make([]webtpl.BMCJob, count)
	for i := range count {
		jobs[i] = webtpl.BMCJob{
			Name:      "job-" + string(rune('a'+i%26)),
			Namespace: "default",
		}
	}
	return jobs
}

func makeBMCTaskList(count int) []webtpl.BMCTask {
	tasks := make([]webtpl.BMCTask, count)
	for i := range count {
		tasks[i] = webtpl.BMCTask{
			Name:      "task-" + string(rune('a'+i%26)),
			Namespace: "default",
		}
	}
	return tasks
}
