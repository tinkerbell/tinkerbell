// Package webhttp provides HTTP handlers for the Tinkerbell web UI.
package webhttp

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	bmcv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Common constants.
const (
	DefaultItemsPerPage = 10
	MaxItemsPerPage     = 100
	statusUnknown       = "Unknown"
	statusCompleted     = "Completed"
	statusFailed        = "Failed"
	statusRunning       = "Running"
	htmxRequestTrue     = "true"
	// ContextKeyBaseURL is the key used to store the URL prefix in Gin context.
	ContextKeyBaseURL = "baseURL"
)

// ValidateItemsPerPage validates and normalizes the items per page value.
// Returns DefaultItemsPerPage if the value is invalid, less than 1, or greater than MaxItemsPerPage.
func ValidateItemsPerPage(itemsPerPage int) int {
	if itemsPerPage < 1 {
		return DefaultItemsPerPage
	}
	if itemsPerPage > MaxItemsPerPage {
		return MaxItemsPerPage
	}
	return itemsPerPage
}

// GetBaseURL retrieves the URL prefix from the Gin context.
// Returns empty string if not set (for backwards compatibility).
func GetBaseURL(c *gin.Context) string {
	if baseURL, exists := c.Get(ContextKeyBaseURL); exists {
		if s, ok := baseURL.(string); ok {
			return s
		}
	}
	return ""
}

// KubeClient wraps a controller-runtime client for Kubernetes operations.
type KubeClient struct {
	client.Client
	clientset *kubernetes.Clientset
}

// NewKubeClientFromTokenAndServer creates a Kubernetes client using JWT token and API server URL.
func NewKubeClientFromTokenAndServer(token, apiServer string, insecureSkipVerify bool) (*KubeClient, error) {
	config := &rest.Config{
		Host:        apiServer,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: insecureSkipVerify,
		},
	}

	return NewKubeClientFromRestConfig(config)
}

// NewKubeClientFromRestConfig creates a Kubernetes client from an existing REST config.
func NewKubeClientFromRestConfig(config *rest.Config) (*KubeClient, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core types to scheme: %w", err)
	}
	if err := api.AddToSchemeTinkerbell(scheme); err != nil {
		return nil, fmt.Errorf("failed to add tinkerbell types to scheme: %w", err)
	}
	if err := api.AddToSchemeBMC(scheme); err != nil {
		return nil, fmt.Errorf("failed to add bmc types to scheme: %w", err)
	}

	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &KubeClient{Client: c, clientset: clientset}, nil
}

// AuthorizationV1 returns the authorization client interface.
func (k *KubeClient) AuthorizationV1() kubernetes.Interface {
	return k.clientset
}

// ListNamespaces returns all namespace names that the user has access to.
func (k *KubeClient) ListNamespaces(ctx context.Context) ([]string, error) {
	var nsList corev1.NamespaceList
	if err := k.List(ctx, &nsList); err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}
	return names, nil
}

// ListHardware returns all hardware resources, optionally filtered by namespace.
func (k *KubeClient) ListHardware(ctx context.Context, namespace string) (*tinkv1alpha1.HardwareList, error) {
	var hwList tinkv1alpha1.HardwareList
	opts := []client.ListOption{}
	if namespace != "" && namespace != templates.AllNamespace {
		opts = append(opts, client.InNamespace(namespace))
	}

	if err := k.List(ctx, &hwList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list hardware: %w", err)
	}
	return &hwList, nil
}

// GetHardware returns a specific hardware resource.
func (k *KubeClient) GetHardware(ctx context.Context, namespace, name string) (*tinkv1alpha1.Hardware, error) {
	var hw tinkv1alpha1.Hardware
	if err := k.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &hw); err != nil {
		return nil, fmt.Errorf("failed to get hardware: %w", err)
	}
	return &hw, nil
}

// ListWorkflows returns all workflow resources, optionally filtered by namespace.
func (k *KubeClient) ListWorkflows(ctx context.Context, namespace string) (*tinkv1alpha1.WorkflowList, error) {
	var wfList tinkv1alpha1.WorkflowList
	opts := []client.ListOption{}
	if namespace != "" && namespace != templates.AllNamespace {
		opts = append(opts, client.InNamespace(namespace))
	}

	if err := k.List(ctx, &wfList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	return &wfList, nil
}

// GetWorkflow returns a specific workflow resource.
func (k *KubeClient) GetWorkflow(ctx context.Context, namespace, name string) (*tinkv1alpha1.Workflow, error) {
	var wf tinkv1alpha1.Workflow
	if err := k.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &wf); err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	return &wf, nil
}

// ListTemplates returns all template resources, optionally filtered by namespace.
func (k *KubeClient) ListTemplates(ctx context.Context, namespace string) (*tinkv1alpha1.TemplateList, error) {
	var tplList tinkv1alpha1.TemplateList
	opts := []client.ListOption{}
	if namespace != "" && namespace != templates.AllNamespace {
		opts = append(opts, client.InNamespace(namespace))
	}

	if err := k.List(ctx, &tplList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}
	return &tplList, nil
}

// GetTemplate returns a specific template resource.
func (k *KubeClient) GetTemplate(ctx context.Context, namespace, name string) (*tinkv1alpha1.Template, error) {
	var tpl tinkv1alpha1.Template
	if err := k.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &tpl); err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	return &tpl, nil
}

// ListWorkflowRuleSets returns all workflowruleset resources, optionally filtered by namespace.
func (k *KubeClient) ListWorkflowRuleSets(ctx context.Context, namespace string) (*tinkv1alpha1.WorkflowRuleSetList, error) {
	var rsList tinkv1alpha1.WorkflowRuleSetList
	opts := []client.ListOption{}
	if namespace != "" && namespace != templates.AllNamespace {
		opts = append(opts, client.InNamespace(namespace))
	}

	if err := k.List(ctx, &rsList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list workflowrulesets: %w", err)
	}
	return &rsList, nil
}

// GetWorkflowRuleSet returns a specific workflowruleset resource.
func (k *KubeClient) GetWorkflowRuleSet(ctx context.Context, namespace, name string) (*tinkv1alpha1.WorkflowRuleSet, error) {
	var rs tinkv1alpha1.WorkflowRuleSet
	if err := k.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &rs); err != nil {
		return nil, fmt.Errorf("failed to get workflowruleset: %w", err)
	}
	return &rs, nil
}

// ListBMCMachines returns all BMC machine resources, optionally filtered by namespace.
func (k *KubeClient) ListBMCMachines(ctx context.Context, namespace string) (*bmcv1alpha1.MachineList, error) {
	var machineList bmcv1alpha1.MachineList
	opts := []client.ListOption{}
	if namespace != "" && namespace != templates.AllNamespace {
		opts = append(opts, client.InNamespace(namespace))
	}

	if err := k.List(ctx, &machineList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list BMC machines: %w", err)
	}
	return &machineList, nil
}

// GetBMCMachine returns a specific BMC machine resource.
func (k *KubeClient) GetBMCMachine(ctx context.Context, namespace, name string) (*bmcv1alpha1.Machine, error) {
	var machine bmcv1alpha1.Machine
	if err := k.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &machine); err != nil {
		return nil, fmt.Errorf("failed to get BMC machine: %w", err)
	}
	return &machine, nil
}

// ListBMCJobs returns all BMC job resources, optionally filtered by namespace.
func (k *KubeClient) ListBMCJobs(ctx context.Context, namespace string) (*bmcv1alpha1.JobList, error) {
	var jobList bmcv1alpha1.JobList
	opts := []client.ListOption{}
	if namespace != "" && namespace != templates.AllNamespace {
		opts = append(opts, client.InNamespace(namespace))
	}

	if err := k.List(ctx, &jobList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list BMC jobs: %w", err)
	}
	return &jobList, nil
}

// GetBMCJob returns a specific BMC job resource.
func (k *KubeClient) GetBMCJob(ctx context.Context, namespace, name string) (*bmcv1alpha1.Job, error) {
	var job bmcv1alpha1.Job
	if err := k.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &job); err != nil {
		return nil, fmt.Errorf("failed to get BMC job: %w", err)
	}
	return &job, nil
}

// ListBMCTasks returns all BMC task resources, optionally filtered by namespace.
func (k *KubeClient) ListBMCTasks(ctx context.Context, namespace string) (*bmcv1alpha1.TaskList, error) {
	var taskList bmcv1alpha1.TaskList
	opts := []client.ListOption{}
	if namespace != "" && namespace != templates.AllNamespace {
		opts = append(opts, client.InNamespace(namespace))
	}

	if err := k.List(ctx, &taskList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list BMC tasks: %w", err)
	}
	return &taskList, nil
}

// GetBMCTask returns a specific BMC task resource.
func (k *KubeClient) GetBMCTask(ctx context.Context, namespace, name string) (*bmcv1alpha1.Task, error) {
	var task bmcv1alpha1.Task
	if err := k.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &task); err != nil {
		return nil, fmt.Errorf("failed to get BMC task: %w", err)
	}
	return &task, nil
}

// Helper functions.

// GetKubeClientFromGinContext gets the KubeClient from the request context (set by AuthMiddleware).
func GetKubeClientFromGinContext(c *gin.Context) (*KubeClient, error) {
	if val, ok := c.Get("kubeClient"); ok {
		if userClient, ok := val.(*KubeClient); ok {
			return userClient, nil
		}
	}
	return nil, fmt.Errorf("no kubernetes client found in context")
}

// IsAuthError checks if an error is an authentication error that should trigger a logout.
// This does NOT include authorization (403 Forbidden) errors - those indicate the user
// is authenticated but lacks permission, and should not cause a logout.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Check for authentication errors only (not authorization/forbidden errors)
	// Authentication errors indicate invalid/expired credentials requiring re-login
	authIndicators := []string{
		"Unauthorized",          // HTTP 401
		"unauthorized",          // HTTP 401
		"provide credentials",   // Missing credentials
		"invalid bearer token",  // Malformed token
		"token is expired",      // Expired token
		"token has expired",     // Expired token
		"certificate",           // TLS cert issues
		"tls",                   // TLS issues
		"x509",                  // Certificate errors
		"authentication failed", // Generic auth failure
		"not authenticated",     // Not authenticated
		"unauthenticated",       // Not authenticated
	}

	for _, indicator := range authIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
}

// HandleAuthError checks if the error is an auth error and redirects to login if needed.
// Returns true if an auth error was detected and handled.
func HandleAuthError(c *gin.Context, err error, log logr.Logger) bool {
	if err == nil {
		return false
	}

	if IsAuthError(err) {
		log.V(1).Info("Authentication/authorization error detected, clearing session",
			"error", err.Error(),
			"path", c.Request.URL.Path,
		)

		// For HTMX requests, send a redirect header
		if IsHTMXRequest(c) {
			c.Header("HX-Redirect", "/login")
			c.Status(401)
			c.Abort()
		} else {
			c.Redirect(302, "/login")
			c.Abort()
		}
		return true
	}

	return false
}

// RenderComponent renders a templ component to the response writer and logs any errors.
func RenderComponent(ctx context.Context, w io.Writer, component templ.Component, log logr.Logger) {
	if err := component.Render(ctx, w); err != nil {
		log.Error(err, "Failed to render component")
	}
}

// IsHTMXRequest checks if the request is an HTMX request.
func IsHTMXRequest(c *gin.Context) bool {
	return c.GetHeader("HX-Request") == htmxRequestTrue
}

// GetHardwareInterfaces extracts all network interfaces from hardware.
func GetHardwareInterfaces(hw tinkv1alpha1.Hardware) []templates.HardwareInterface {
	var interfaces []templates.HardwareInterface
	for _, iface := range hw.Spec.Interfaces {
		if iface.DHCP != nil {
			mac := iface.DHCP.MAC
			ip := ""
			if iface.DHCP.IP != nil {
				ip = iface.DHCP.IP.Address
			}
			if mac != "" || ip != "" {
				interfaces = append(interfaces, templates.HardwareInterface{
					MAC: mac,
					IP:  ip,
				})
			}
		}
	}
	return interfaces
}

// GetHardwareStatus determines hardware status.
func GetHardwareStatus(hw tinkv1alpha1.Hardware) string {
	if hw.Status.State == "provisioning" {
		return "Provisioning"
	}
	if hw.Status.State == "failed" {
		return "Offline"
	}
	return "Online"
}

// GetKubeNamespaces fetches namespaces from the Kubernetes cluster.
// Returns empty list if the user doesn't have permission to list namespaces.
func GetKubeNamespaces(ctx context.Context, c *gin.Context, kubeClient *KubeClient, log logr.Logger) []string {
	if kubeClient == nil {
		return []string{}
	}
	namespaces, err := kubeClient.ListNamespaces(ctx)
	if err != nil {
		log.V(1).Info("Cannot list namespaces - user may have namespace-scoped permissions only", "error", err)

		// For namespace-scoped users, return their service account's namespace
		if saNamespace, exists := c.Get("sa_namespace"); exists {
			if ns, ok := saNamespace.(string); ok && ns != "" {
				return []string{ns}
			}
		}

		return []string{}
	}
	return namespaces
}

// GetSelectedNamespace returns the selected namespace from query params.
// If no namespace is selected, uses smart defaults based on available namespaces.
func GetSelectedNamespace(c *gin.Context, namespaces []string) string {
	selected := c.Query("namespace")

	// If namespace already selected via query param, use it
	if selected != "" {
		return selected
	}

	// If user has exactly one namespace (namespace-scoped), use that namespace
	if len(namespaces) == 1 {
		return namespaces[0]
	}

	// If user has no namespaces or multiple namespaces, return empty (means "All")
	return ""
}

// GetBMCJobStatus determines the status of a BMC job from its conditions.
func GetBMCJobStatus(conditions []bmcv1alpha1.JobCondition) string {
	for _, condition := range conditions {
		if condition.Status != bmcv1alpha1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case bmcv1alpha1.JobCompleted:
			return statusCompleted
		case bmcv1alpha1.JobFailed:
			return statusFailed
		case bmcv1alpha1.JobRunning:
			return statusRunning
		}
	}
	return statusUnknown
}
