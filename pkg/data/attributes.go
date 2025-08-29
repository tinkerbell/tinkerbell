package data

// AgentAttributes are attributes of an Agent.
type AgentAttributes struct {
	CPU               *CPU       `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	Memory            *Memory    `json:"memory,omitempty" yaml:"memory,omitempty"`
	BlockDevices      []*Block   `json:"blockDevices,omitempty" yaml:"blockDevices,omitempty"`
	NetworkInterfaces []*Network `json:"networkInterfaces,omitempty" yaml:"networkInterfaces,omitempty"`
	PCIDevices        []*PCI     `json:"pciDevices,omitempty" yaml:"pciDevices,omitempty"`
	GPUDevices        []*GPU     `json:"gpuDevices,omitempty" yaml:"gpuDevices,omitempty"`
	Chassis           *Chassis   `json:"chassis,omitempty" yaml:"chassis,omitempty"`
	BIOS              *BIOS      `json:"bios,omitempty" yaml:"bios,omitempty"`
	Baseboard         *Baseboard `json:"baseboard,omitempty" yaml:"baseboard,omitempty"`
	Product           *Product   `json:"product,omitempty" yaml:"product,omitempty"`
}

type CPU struct {
	TotalCores   *uint32      `json:"totalCores,omitempty" yaml:"totalCores,omitempty"`
	TotalThreads *uint32      `json:"totalThreads,omitempty" yaml:"totalThreads,omitempty"`
	Processors   []*Processor `json:"processors,omitempty" yaml:"processors,omitempty"`
}

type Processor struct {
	ID           *uint32  `json:"id,omitempty" yaml:"id,omitempty"`
	Cores        *uint32  `json:"cores,omitempty" yaml:"cores,omitempty"`
	Threads      *uint32  `json:"threads,omitempty" yaml:"threads,omitempty"`
	Vendor       *string  `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Model        *string  `json:"model,omitempty" yaml:"model,omitempty"`
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

type Memory struct {
	Total  *string `json:"total,omitempty" yaml:"total,omitempty"`
	Usable *string `json:"usable,omitempty" yaml:"usable,omitempty"`
}

type Block struct {
	Name              *string `json:"name,omitempty" yaml:"name,omitempty"`
	ControllerType    *string `json:"controllerType,omitempty" yaml:"controllerType,omitempty"`
	DriveType         *string `json:"driveType,omitempty" yaml:"driveType,omitempty"`
	Size              *string `json:"size,omitempty" yaml:"size,omitempty"`
	PhysicalBlockSize *string `json:"physicalBlockSize,omitempty" yaml:"physicalBlockSize,omitempty"`
	Vendor            *string `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Model             *string `json:"model,omitempty" yaml:"model,omitempty"`
	Wwn               *string `json:"wwn,omitempty" yaml:"wwn,omitempty"`
}

type Network struct {
	Name                *string  `json:"name,omitempty" yaml:"name,omitempty"`
	Mac                 *string  `json:"mac,omitempty" yaml:"mac,omitempty"`
	Speed               *string  `json:"speed,omitempty" yaml:"speed,omitempty"`
	EnabledCapabilities []string `json:"enabledCapabilities,omitempty" yaml:"enabledCapabilities,omitempty"`
}

type PCI struct {
	Vendor  *string `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Product *string `json:"product,omitempty" yaml:"product,omitempty"`
	Class   *string `json:"class,omitempty" yaml:"class,omitempty"`
	Driver  *string `json:"driver,omitempty" yaml:"driver,omitempty"`
}

type GPU struct {
	Vendor  *string `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Product *string `json:"product,omitempty" yaml:"product,omitempty"`
	Class   *string `json:"class,omitempty" yaml:"class,omitempty"`
	Driver  *string `json:"driver,omitempty" yaml:"driver,omitempty"`
}

type Chassis struct {
	Serial *string `json:"serial,omitempty" yaml:"serial,omitempty"`
	Vendor *string `json:"vendor,omitempty" yaml:"vendor,omitempty"`
}

type BIOS struct {
	Vendor      *string `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Version     *string `json:"version,omitempty" yaml:"version,omitempty"`
	ReleaseDate *string `json:"releaseDate,omitempty" yaml:"releaseDate,omitempty"`
}

type Baseboard struct {
	Vendor       *string `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Product      *string `json:"product,omitempty" yaml:"product,omitempty"`
	Version      *string `json:"version,omitempty" yaml:"version,omitempty"`
	SerialNumber *string `json:"serialNumber,omitempty" yaml:"serialNumber,omitempty"`
}

type Product struct {
	Name         *string `json:"name,omitempty" yaml:"name,omitempty"`
	Vendor       *string `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	SerialNumber *string `json:"serialNumber,omitempty" yaml:"serialNumber,omitempty"`
}

// NewAgentAttributes initializes a new AgentAttributes struct.
func NewAgentAttributes() *AgentAttributes {
	return &AgentAttributes{
		CPU:               &CPU{},
		Memory:            &Memory{},
		BlockDevices:      []*Block{},
		NetworkInterfaces: []*Network{},
		PCIDevices:        []*PCI{},
		GPUDevices:        []*GPU{},
		Chassis:           &Chassis{},
		BIOS:              &BIOS{},
		Baseboard:         &Baseboard{},
		Product:           &Product{},
	}
}
