package webhttp

import (
	"fmt"
	"strconv"

	"github.com/go-logr/logr"

	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	"sigs.k8s.io/yaml"
)

const (
	nameSingularTask    = "Task"
	namePluralTask      = "Tasks"
	nameSingularJob     = "Job"
	namePluralJob       = "Jobs"
	nameSingularMachine = "Machine"
	namePluralMachine   = "Machines"
)

// HandleBMCMachineList handles the Machine list page route.
func HandleBMCMachineList(c *gin.Context, log logr.Logger) {
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

	selectedNamespace := c.Query("namespace")

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 1
	}

	itemsPerPageStr := c.DefaultQuery("per_page", strconv.Itoa(DefaultItemsPerPage))
	itemsPerPage, err := strconv.Atoi(itemsPerPageStr)
	if err != nil || itemsPerPage < 1 {
		itemsPerPage = DefaultItemsPerPage
	}

	var machines []templates.BMCMachine

	machineList, err := kubeClient.ListBMCMachines(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info(fmt.Sprintf("Failed to list %s", namePluralMachine), "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, machine := range machineList.Items {
			contactable := statusUnknown
			for _, condition := range machine.Status.Conditions {
				if condition.Type == bmc.Contactable {
					contactable = string(condition.Status)
					break
				}
			}

			webMachine := templates.BMCMachine{
				Name:        machine.Name,
				Namespace:   machine.Namespace,
				PowerState:  string(machine.Status.Power),
				Contactable: contactable,
				Endpoint:    machine.Spec.Connection.Host,
				CreatedAt:   machine.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			machines = append(machines, webMachine)
		}
	}

	machinePageData := GetPaginatedBMCMachines(machines, page, itemsPerPage)

	if IsHTMXRequest(c) {
		component := templates.BMCMachineTableContent(machinePageData)
		c.Header("Content-Type", "text/html")
		RenderComponent(c.Request.Context(), c.Writer, component, log)
		return
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.BMCMachinePage(cfg, machinePageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleBMCMachineData handles the Machine data endpoint (HTMX partial).
func HandleBMCMachineData(c *gin.Context, log logr.Logger) {
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
	itemsPerPage, err := strconv.Atoi(itemsPerPageStr)
	if err != nil || itemsPerPage < 1 {
		itemsPerPage = DefaultItemsPerPage
	}

	var machines []templates.BMCMachine

	machineList, err := kubeClient.ListBMCMachines(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info(fmt.Sprintf("Failed to list %s", namePluralMachine), "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, machine := range machineList.Items {
			contactable := statusUnknown
			for _, condition := range machine.Status.Conditions {
				if condition.Type == bmc.Contactable {
					contactable = string(condition.Status)
					break
				}
			}

			webMachine := templates.BMCMachine{
				Name:        machine.Name,
				Namespace:   machine.Namespace,
				PowerState:  string(machine.Status.Power),
				Contactable: contactable,
				Endpoint:    machine.Spec.Connection.Host,
				CreatedAt:   machine.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			machines = append(machines, webMachine)
		}
	}

	machinePageData := GetPaginatedBMCMachines(machines, page, itemsPerPage)

	component := templates.BMCMachineTableContent(machinePageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleBMCMachineDetail handles the Machine detail page route.
func HandleBMCMachineDetail(c *gin.Context, log logr.Logger) {
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
	namespace := c.Param("namespace")
	name := c.Param("name")

	namespaces := GetKubeNamespaces(ctx, c, kubeClient, log)

	machine, err := kubeClient.GetBMCMachine(ctx, namespace, name)
	if err != nil {
		log.V(1).Info(fmt.Sprintf("Failed to fetch %s", nameSingularMachine), "namespace", namespace, "name", name, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
		cfg := templates.PageConfig{
			BaseURL:    GetBaseURL(c),
			Namespaces: namespaces,
		}
		component := templates.NotFoundPage(cfg, nameSingularMachine, name, namespace, "/bmc/machines", namePluralMachine, fmt.Sprintf("This %s may have been deleted or the reference is incorrect.", nameSingularMachine))
		c.Header("Content-Type", "text/html")
		c.Status(404)
		RenderComponent(ctx, c.Writer, component, log)
		return
	}

	yamlBytes, _ := yaml.Marshal(machine)
	specYAML, _ := yaml.Marshal(&machine.Spec)
	statusYAML, _ := yaml.Marshal(&machine.Status)

	contactable := statusUnknown
	for _, condition := range machine.Status.Conditions {
		if condition.Type == bmc.Contactable {
			contactable = string(condition.Status)
			break
		}
	}

	machineDetail := templates.BMCMachineDetail{
		Name:        machine.Name,
		Namespace:   machine.Namespace,
		PowerState:  string(machine.Status.Power),
		Contactable: contactable,
		Endpoint:    machine.Spec.Connection.Host,
		CreatedAt:   machine.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
		Labels:      machine.Labels,
		Annotations: machine.Annotations,
		SpecYAML:    string(specYAML),
		StatusYAML:  string(statusYAML),
		YAML:        string(yamlBytes),
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.BMCMachineDetailPage(cfg, machineDetail)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleBMCJobList handles the Job list page route.
func HandleBMCJobList(c *gin.Context, log logr.Logger) {
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

	selectedNamespace := c.Query("namespace")

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 1
	}

	itemsPerPageStr := c.DefaultQuery("per_page", strconv.Itoa(DefaultItemsPerPage))
	itemsPerPage, err := strconv.Atoi(itemsPerPageStr)
	if err != nil || itemsPerPage < 1 {
		itemsPerPage = DefaultItemsPerPage
	}

	var jobs []templates.BMCJob

	jobList, err := kubeClient.ListBMCJobs(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info(fmt.Sprintf("Failed to list %s", namePluralJob), "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, job := range jobList.Items {
			webJob := templates.BMCJob{
				Name:        job.Name,
				Namespace:   job.Namespace,
				MachineRef:  job.Spec.MachineRef.Namespace + "/" + job.Spec.MachineRef.Name,
				Status:      GetBMCJobStatus(job.Status.Conditions),
				CompletedAt: "",
				CreatedAt:   job.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			jobs = append(jobs, webJob)
		}
	}

	jobPageData := GetPaginatedBMCJobs(jobs, page, itemsPerPage)

	if IsHTMXRequest(c) {
		component := templates.BMCJobTableContent(jobPageData)
		c.Header("Content-Type", "text/html")
		RenderComponent(c.Request.Context(), c.Writer, component, log)
		return
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.BMCJobPage(cfg, jobPageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleBMCJobData handles the Job data endpoint (HTMX partial).
func HandleBMCJobData(c *gin.Context, log logr.Logger) {
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
	itemsPerPage, err := strconv.Atoi(itemsPerPageStr)
	if err != nil || itemsPerPage < 1 {
		itemsPerPage = DefaultItemsPerPage
	}

	var jobs []templates.BMCJob

	jobList, err := kubeClient.ListBMCJobs(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info(fmt.Sprintf("Failed to list %s", namePluralJob), "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, job := range jobList.Items {
			webJob := templates.BMCJob{
				Name:        job.Name,
				Namespace:   job.Namespace,
				MachineRef:  job.Spec.MachineRef.Namespace + "/" + job.Spec.MachineRef.Name,
				Status:      GetBMCJobStatus(job.Status.Conditions),
				CompletedAt: "",
				CreatedAt:   job.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			jobs = append(jobs, webJob)
		}
	}

	jobPageData := GetPaginatedBMCJobs(jobs, page, itemsPerPage)

	component := templates.BMCJobTableContent(jobPageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleBMCJobDetail handles the Job detail page route.
func HandleBMCJobDetail(c *gin.Context, log logr.Logger) {
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
	namespace := c.Param("namespace")
	name := c.Param("name")

	namespaces := GetKubeNamespaces(ctx, c, kubeClient, log)

	job, err := kubeClient.GetBMCJob(ctx, namespace, name)
	if err != nil {
		log.V(1).Info(fmt.Sprintf("Failed to fetch %s", nameSingularJob), "namespace", namespace, "name", name, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
		cfg := templates.PageConfig{
			BaseURL:    GetBaseURL(c),
			Namespaces: namespaces,
		}
		component := templates.NotFoundPage(cfg, nameSingularJob, name, namespace, "/bmc/jobs", namePluralJob, fmt.Sprintf("This %s may have been deleted.", nameSingularJob))
		c.Header("Content-Type", "text/html")
		c.Status(404)
		RenderComponent(ctx, c.Writer, component, log)
		return
	}

	yamlBytes, _ := yaml.Marshal(job)
	specYAML, _ := yaml.Marshal(&job.Spec)
	statusYAML, _ := yaml.Marshal(&job.Status)

	jobDetail := templates.BMCJobDetail{
		Name:        job.Name,
		Namespace:   job.Namespace,
		MachineRef:  job.Spec.MachineRef.Namespace + "/" + job.Spec.MachineRef.Name,
		Status:      GetBMCJobStatus(job.Status.Conditions),
		CompletedAt: "",
		CreatedAt:   job.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
		Labels:      job.Labels,
		Annotations: job.Annotations,
		SpecYAML:    string(specYAML),
		StatusYAML:  string(statusYAML),
		YAML:        string(yamlBytes),
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.BMCJobDetailPage(cfg, jobDetail)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleBMCTaskList handles the Task list page route.
func HandleBMCTaskList(c *gin.Context, log logr.Logger) {
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

	selectedNamespace := c.Query("namespace")

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 1
	}

	itemsPerPageStr := c.DefaultQuery("per_page", strconv.Itoa(DefaultItemsPerPage))
	itemsPerPage, err := strconv.Atoi(itemsPerPageStr)
	if err != nil || itemsPerPage < 1 {
		itemsPerPage = DefaultItemsPerPage
	}

	var tasks []templates.BMCTask

	taskList, err := kubeClient.ListBMCTasks(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list Tasks", "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, task := range taskList.Items {
			status := statusUnknown
			completedAt := ""
			for _, condition := range task.Status.Conditions {
				if condition.Type == bmc.TaskCompleted && condition.Status == bmc.ConditionTrue {
					status = statusCompleted
					if task.Status.CompletionTime != nil {
						completedAt = task.Status.CompletionTime.Format("2006-01-02 15:04:05")
					}
					break
				} else if condition.Type == bmc.TaskFailed && condition.Status == bmc.ConditionTrue {
					status = statusFailed
					break
				}
			}

			taskType := "Unknown"
			if task.Spec.Task.PowerAction != nil {
				taskType = "Power"
			}

			webTask := templates.BMCTask{
				Name:        task.Name,
				Namespace:   task.Namespace,
				JobRef:      "N/A",
				TaskType:    taskType,
				Status:      status,
				CompletedAt: completedAt,
				CreatedAt:   task.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			tasks = append(tasks, webTask)
		}
	}

	taskPageData := GetPaginatedBMCTasks(tasks, page, itemsPerPage)

	if IsHTMXRequest(c) {
		component := templates.BMCTaskTableContent(taskPageData)
		c.Header("Content-Type", "text/html")
		RenderComponent(c.Request.Context(), c.Writer, component, log)
		return
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.BMCTaskPage(cfg, taskPageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleBMCTaskData handles the BMC task data endpoint (HTMX partial).
func HandleBMCTaskData(c *gin.Context, log logr.Logger) {
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
	itemsPerPage, err := strconv.Atoi(itemsPerPageStr)
	if err != nil || itemsPerPage < 1 {
		itemsPerPage = DefaultItemsPerPage
	}

	var tasks []templates.BMCTask

	taskList, err := kubeClient.ListBMCTasks(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list Tasks", "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
	} else {
		for _, task := range taskList.Items {
			status := statusUnknown
			completedAt := ""
			for _, condition := range task.Status.Conditions {
				if condition.Type == bmc.TaskCompleted && condition.Status == bmc.ConditionTrue {
					status = statusCompleted
					if task.Status.CompletionTime != nil {
						completedAt = task.Status.CompletionTime.Format("2006-01-02 15:04:05")
					}
					break
				} else if condition.Type == bmc.TaskFailed && condition.Status == bmc.ConditionTrue {
					status = statusFailed
					break
				}
			}

			taskType := "Unknown"
			if task.Spec.Task.PowerAction != nil {
				taskType = "Power"
			}

			webTask := templates.BMCTask{
				Name:        task.Name,
				Namespace:   task.Namespace,
				JobRef:      "N/A",
				TaskType:    taskType,
				Status:      status,
				CompletedAt: completedAt,
				CreatedAt:   task.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			tasks = append(tasks, webTask)
		}
	}

	taskPageData := GetPaginatedBMCTasks(tasks, page, itemsPerPage)

	component := templates.BMCTaskTableContent(taskPageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleBMCTaskDetail handles the Task detail page route.
func HandleBMCTaskDetail(c *gin.Context, log logr.Logger) {
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
	namespace := c.Param("namespace")
	name := c.Param("name")

	namespaces := GetKubeNamespaces(ctx, c, kubeClient, log)

	task, err := kubeClient.GetBMCTask(ctx, namespace, name)
	if err != nil {
		log.V(1).Info(fmt.Sprintf("Failed to fetch %s", nameSingularTask), "namespace", namespace, "name", name, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
		cfg := templates.PageConfig{
			BaseURL:    GetBaseURL(c),
			Namespaces: namespaces,
		}
		component := templates.NotFoundPage(cfg, nameSingularTask, name, namespace, "/bmc/tasks", namePluralTask, fmt.Sprintf("This %s may have been deleted.", nameSingularTask))
		c.Header("Content-Type", "text/html")
		c.Status(404)
		RenderComponent(ctx, c.Writer, component, log)
		return
	}

	yamlBytes, _ := yaml.Marshal(task)
	specYAML, _ := yaml.Marshal(&task.Spec)
	statusYAML, _ := yaml.Marshal(&task.Status)

	status := statusUnknown
	completedAt := ""
	for _, condition := range task.Status.Conditions {
		if condition.Type == bmc.TaskCompleted && condition.Status == bmc.ConditionTrue {
			status = statusCompleted
			if task.Status.CompletionTime != nil {
				completedAt = task.Status.CompletionTime.Format("2006-01-02 15:04:05")
			}
			break
		} else if condition.Type == bmc.TaskFailed && condition.Status == bmc.ConditionTrue {
			status = statusFailed
			break
		}
	}

	taskType := "Unknown"
	if task.Spec.Task.PowerAction != nil {
		taskType = "Power"
	}

	taskDetail := templates.BMCTaskDetail{
		Name:        task.Name,
		Namespace:   task.Namespace,
		JobRef:      "N/A",
		TaskType:    taskType,
		Status:      status,
		CompletedAt: completedAt,
		CreatedAt:   task.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
		Labels:      task.Labels,
		Annotations: task.Annotations,
		SpecYAML:    string(specYAML),
		StatusYAML:  string(statusYAML),
		YAML:        string(yamlBytes),
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.BMCTaskDetailPage(cfg, taskDetail)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}
