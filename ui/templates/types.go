package templates

// PageConfig holds common page configuration.
type PageConfig struct {
	BaseURL    string   // URL prefix for all routes (e.g., "/ui")
	Namespaces []string // Available namespaces
}

// NavItem represents a navigation menu item.
type NavItem struct {
	Name string
	Icon string
	Href string
}

// Hardware represents a hardware resource in the list view.
type Hardware struct {
	Name        string
	Namespace   string
	Description string
	MAC         string
	IPv4Address string
	Status      string
	CreatedAt   string
}

// Workflow represents a workflow resource in the list view.
type Workflow struct {
	Name        string
	Namespace   string
	TemplateRef string
	State       string
	Task        string
	Action      string
	Agent       string
	CreatedAt   string
}

// WorkflowRuleSet represents a workflowruleset resource in the list view.
type WorkflowRuleSet struct {
	Name        string
	Namespace   string
	Rules       string
	TemplateRef string
	CreatedAt   string
}

// Template represents a template resource in the list view.
type Template struct {
	Name      string
	Namespace string
	State     string
	Data      string
	CreatedAt string
}

// BMCMachine represents a BMC machine resource in the list view.
type BMCMachine struct {
	Name        string
	Namespace   string
	PowerState  string
	Contactable string
	Endpoint    string
	CreatedAt   string
}

// BMCJob represents a BMC job resource in the list view.
type BMCJob struct {
	Name        string
	Namespace   string
	MachineRef  string
	Status      string
	CompletedAt string
	CreatedAt   string
}

// BMCTask represents a BMC task resource in the list view.
type BMCTask struct {
	Name        string
	Namespace   string
	JobRef      string
	TaskType    string
	Status      string
	CompletedAt string
	CreatedAt   string
}

// Permission represents a Kubernetes RBAC permission for a Tinkerbell resource.
type Permission struct {
	Resource  string
	APIGroup  string
	Namespace string   // Namespace scope (empty means cluster-wide)
	Verbs     []string // Allowed verbs for this resource
}

// ResourceInfo represents a Tinkerbell resource for the permissions loading page.
type ResourceInfo struct {
	Resource string
	APIGroup string
}

// PaginationData holds pagination state for list views.
type PaginationData struct {
	CurrentPage  int
	TotalPages   int
	TotalItems   int
	ItemsPerPage int
	StartItem    int
	EndItem      int
	ResourcePath string // e.g., "/hardware", "/workflows", "/templates"
	TargetID     string // e.g., "#hardware-content", "#workflow-content"
}

// HardwarePageData is the data for the hardware list page.
type HardwarePageData struct {
	Hardware   []Hardware
	Pagination PaginationData
}

// WorkflowPageData is the data for the workflow list page.
type WorkflowPageData struct {
	Workflows  []Workflow
	Pagination PaginationData
}

// WorkflowRuleSetPageData is the data for the workflowruleset list page.
type WorkflowRuleSetPageData struct {
	RuleSets   []WorkflowRuleSet
	Pagination PaginationData
}

// TemplatePageData is the data for the template list page.
type TemplatePageData struct {
	Templates  []Template
	Pagination PaginationData
}

// BMCMachinePageData is the data for the BMC machine list page.
type BMCMachinePageData struct {
	Machines   []BMCMachine
	Pagination PaginationData
}

// BMCJobPageData is the data for the BMC job list page.
type BMCJobPageData struct {
	Jobs       []BMCJob
	Pagination PaginationData
}

// BMCTaskPageData is the data for the BMC task list page.
type BMCTaskPageData struct {
	Tasks      []BMCTask
	Pagination PaginationData
}

// HardwareInterface represents a single network interface.
type HardwareInterface struct {
	MAC string
	IP  string
}

// AgentProcessor represents a processor from agent attributes.
type AgentProcessor struct {
	ID           int      `json:"id"`
	Cores        int      `json:"cores"`
	Threads      int      `json:"threads"`
	Vendor       string   `json:"vendor"`
	Model        string   `json:"model"`
	Capabilities []string `json:"capabilities"`
}

// AgentCPU represents CPU information from agent attributes.
type AgentCPU struct {
	TotalCores   int              `json:"totalCores"`
	TotalThreads int              `json:"totalThreads"`
	Processors   []AgentProcessor `json:"processors"`
}

// AgentMemory represents memory information from agent attributes.
type AgentMemory struct {
	Total  string `json:"total"`
	Usable string `json:"usable"`
}

// AgentBlockDevice represents a block device from agent attributes.
type AgentBlockDevice struct {
	Name              string `json:"name"`
	Size              string `json:"size"`
	ControllerType    string `json:"controllerType"`
	DriveType         string `json:"driveType"`
	PhysicalBlockSize string `json:"physicalBlockSize"`
	Vendor            string `json:"vendor"`
	Model             string `json:"model"`
	WWN               string `json:"wwn"`
	SerialNumber      string `json:"serialNumber"`
}

// AgentNetworkInterface represents a network interface from agent attributes.
type AgentNetworkInterface struct {
	Name                string   `json:"name"`
	MAC                 string   `json:"mac"`
	Speed               string   `json:"speed"`
	EnabledCapabilities []string `json:"enabledCapabilities"`
}

// AgentPCIDevice represents a PCI device from agent attributes.
type AgentPCIDevice struct {
	Vendor  string `json:"vendor"`
	Product string `json:"product"`
	Class   string `json:"class"`
	Driver  string `json:"driver"`
}

// AgentGPUDevice represents a GPU device from agent attributes.
type AgentGPUDevice struct {
	Vendor  string `json:"vendor"`
	Product string `json:"product"`
	Class   string `json:"class"`
	Driver  string `json:"driver"`
}

// AgentChassis represents chassis information from agent attributes.
type AgentChassis struct {
	Serial string `json:"serial"`
	Vendor string `json:"vendor"`
}

// AgentBIOS represents BIOS information from agent attributes.
type AgentBIOS struct {
	Vendor      string `json:"vendor"`
	Version     string `json:"version"`
	ReleaseDate string `json:"releaseDate"`
}

// AgentBaseboard represents baseboard information from agent attributes.
type AgentBaseboard struct {
	Vendor       string `json:"vendor"`
	Product      string `json:"product"`
	Version      string `json:"version"`
	SerialNumber string `json:"serialNumber"`
}

// AgentProduct represents product information from agent attributes.
type AgentProduct struct {
	Name         string `json:"name"`
	Vendor       string `json:"vendor"`
	SerialNumber string `json:"serialNumber"`
}

// AgentAttributes represents the agent-attributes annotation data.
type AgentAttributes struct {
	CPU               AgentCPU                `json:"cpu"`
	Memory            AgentMemory             `json:"memory"`
	BlockDevices      []AgentBlockDevice      `json:"blockDevices"`
	NetworkInterfaces []AgentNetworkInterface `json:"networkInterfaces"`
	PCIDevices        []AgentPCIDevice        `json:"pciDevices"`
	GPUDevices        []AgentGPUDevice        `json:"gpuDevices"`
	Chassis           AgentChassis            `json:"chassis"`
	BIOS              AgentBIOS               `json:"bios"`
	Baseboard         AgentBaseboard          `json:"baseboard"`
	Product           AgentProduct            `json:"product"`
}

// HardwareDetail is the data for the hardware detail page.
type HardwareDetail struct {
	Name            string
	Namespace       string
	Interfaces      []HardwareInterface
	Status          string
	CreatedAt       string
	Labels          map[string]string
	Annotations     map[string]string
	AgentAttributes *AgentAttributes
	SpecYAML        string
	StatusYAML      string
	YAML            string
}

// WorkflowDetail is the data for the workflow detail page.
type WorkflowDetail struct {
	Name              string
	Namespace         string
	TemplateRef       string
	HardwareRef       string
	State             string
	Task              string
	Action            string
	Agent             string
	TemplateRendering string
	CreatedAt         string
	Labels            map[string]string
	Annotations       map[string]string
	SpecYAML          string
	StatusYAML        string
	YAML              string
}

// TemplateDetail is the data for the template detail page.
type TemplateDetail struct {
	Name        string
	Namespace   string
	State       string
	Data        string
	CreatedAt   string
	Labels      map[string]string
	Annotations map[string]string
	SpecYAML    string
	StatusYAML  string
	YAML        string
}

// WorkflowRuleSetDetail is the data for the workflowruleset detail page.
type WorkflowRuleSetDetail struct {
	Name              string
	Namespace         string
	YAMLData          string
	CreatedAt         string
	Labels            map[string]string
	Annotations       map[string]string
	Rules             []string
	TemplateRef       string
	WorkflowNamespace string
	WorkflowDisabled  bool
	AddAttributes     bool
	AgentValue        string
}

// BMCMachineDetail is the data for the BMC machine detail page.
type BMCMachineDetail struct {
	Name        string
	Namespace   string
	PowerState  string
	Contactable string
	Endpoint    string
	CreatedAt   string
	Labels      map[string]string
	Annotations map[string]string
	SpecYAML    string
	StatusYAML  string
	YAML        string
}

// BMCJobDetail is the data for the BMC job detail page.
type BMCJobDetail struct {
	Name        string
	Namespace   string
	MachineRef  string
	Status      string
	CompletedAt string
	CreatedAt   string
	Labels      map[string]string
	Annotations map[string]string
	SpecYAML    string
	StatusYAML  string
	YAML        string
}

// BMCTaskDetail is the data for the BMC task detail page.
type BMCTaskDetail struct {
	Name        string
	Namespace   string
	JobRef      string
	TaskType    string
	Status      string
	CompletedAt string
	CreatedAt   string
	Labels      map[string]string
	Annotations map[string]string
	SpecYAML    string
	StatusYAML  string
	YAML        string
}

// InfoRow represents a single row in a NameValueTable.
type InfoRow struct {
	Name  string
	Value string
	Link  string // If set, Value is rendered as a hyperlink to this URL.
	Hide  bool
}

// SchemaField represents a field in a CRD schema for display.
type SchemaField struct {
	Name        string        // Field name
	Type        string        // Field type (string, integer, boolean, object, array)
	Description string        // Field description from CRD
	Required    bool          // Whether the field is required
	Deprecated  bool          // Whether the field is deprecated
	Children    []SchemaField // Nested fields for objects/arrays
	Pattern     string        // Regex pattern for validation (if any)
	Enum        []string      // Allowed enum values (if any)
	Default     string        // Default value (if any)
	Format      string        // Format hint (date-time, int64, uri, etc.)
}

// CRDInfo represents a Custom Resource Definition for display on the dashboard.
type CRDInfo struct {
	Kind         string        // Resource kind (e.g., "Hardware")
	Plural       string        // Plural name (e.g., "hardware")
	Group        string        // API group (e.g., "tinkerbell.org")
	Version      string        // API version (e.g., "v1alpha1")
	Description  string        // CRD description
	Route        string        // Web UI route (empty if no UI exists)
	SpecFields   []SchemaField // Top-level spec fields
	StatusFields []SchemaField // Top-level status fields
}

// CRDGroup represents a group of CRDs for display.
type CRDGroup struct {
	Name string    // Group name (e.g., "tinkerbell.org")
	CRDs []CRDInfo // CRDs in this group
}

// DashboardData is the data for the landing page/dashboard.
type DashboardData struct {
	Groups []CRDGroup // CRDs grouped by API group
}
