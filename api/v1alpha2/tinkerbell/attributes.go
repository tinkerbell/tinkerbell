package tinkerbell

// AgentAttributes are attributes of an Agent.
type AgentAttributes struct {
	CPU               *CPU       `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	Memory            *Memory    `json:"memory,omitempty" yaml:"memory,omitempty"`
	BlockDevices      *Block     `json:"blockDevices,omitempty" yaml:"blockDevices,omitempty"`
	NetworkInterfaces *Network   `json:"networkInterfaces,omitempty" yaml:"networkInterfaces,omitempty"`
	PCIDevices        *PCI       `json:"pciDevices,omitempty" yaml:"pciDevices,omitempty"`
	GPUDevices        *GPU       `json:"gpuDevices,omitempty" yaml:"gpuDevices,omitempty"`
	Chassis           *Chassis   `json:"chassis,omitempty" yaml:"chassis,omitempty"`
	BIOS              *BIOS      `json:"bios,omitempty" yaml:"bios,omitempty"`
	Baseboard         *Baseboard `json:"baseboard,omitempty" yaml:"baseboard,omitempty"`
	Product           *Product   `json:"product,omitempty" yaml:"product,omitempty"`
}

type CPU struct {
	TotalCores   FieldPattern `json:"totalCores,omitempty" yaml:"totalCores,omitempty"`
	TotalThreads FieldPattern `json:"totalThreads,omitempty" yaml:"totalThreads,omitempty"`
	Processors   *Processor   `json:"processors,omitempty" yaml:"processors,omitempty"`
}

type Processor struct {
	ID           FieldPattern `json:"id,omitempty" yaml:"id,omitempty"`
	Cores        FieldPattern `json:"cores,omitempty" yaml:"cores,omitempty"`
	Threads      FieldPattern `json:"threads,omitempty" yaml:"threads,omitempty"`
	Vendor       FieldPattern `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Model        FieldPattern `json:"model,omitempty" yaml:"model,omitempty"`
	Capabilities FieldPattern `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

type Memory struct {
	Total  FieldPattern `json:"total,omitempty" yaml:"total,omitempty"`
	Usable FieldPattern `json:"usable,omitempty" yaml:"usable,omitempty"`
}

type Block struct {
	Name              FieldPattern `json:"name,omitempty" yaml:"name,omitempty"`
	ControllerType    FieldPattern `json:"controllerType,omitempty" yaml:"controllerType,omitempty"`
	DriveType         FieldPattern `json:"driveType,omitempty" yaml:"driveType,omitempty"`
	Size              FieldPattern `json:"size,omitempty" yaml:"size,omitempty"`
	PhysicalBlockSize FieldPattern `json:"physicalBlockSize,omitempty" yaml:"physicalBlockSize,omitempty"`
	Vendor            FieldPattern `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Model             FieldPattern `json:"model,omitempty" yaml:"model,omitempty"`
	WWN               FieldPattern `json:"wwn,omitempty" yaml:"wwn,omitempty"`
	SerialNumber      FieldPattern `json:"serialNumber,omitempty" yaml:"serialNumber,omitempty"`
}

type Network struct {
	Name                FieldPattern `json:"name,omitempty" yaml:"name,omitempty"`
	Mac                 FieldPattern `json:"mac,omitempty" yaml:"mac,omitempty"`
	Speed               FieldPattern `json:"speed,omitempty" yaml:"speed,omitempty"`
	EnabledCapabilities FieldPattern `json:"enabledCapabilities,omitempty" yaml:"enabledCapabilities,omitempty"`
}

type PCI struct {
	Vendor  FieldPattern `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Product FieldPattern `json:"product,omitempty" yaml:"product,omitempty"`
	Class   FieldPattern `json:"class,omitempty" yaml:"class,omitempty"`
	Driver  FieldPattern `json:"driver,omitempty" yaml:"driver,omitempty"`
}

type GPU struct {
	Vendor  FieldPattern `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Product FieldPattern `json:"product,omitempty" yaml:"product,omitempty"`
	Class   FieldPattern `json:"class,omitempty" yaml:"class,omitempty"`
	Driver  FieldPattern `json:"driver,omitempty" yaml:"driver,omitempty"`
}

type Chassis struct {
	Serial FieldPattern `json:"serial,omitempty" yaml:"serial,omitempty"`
	Vendor FieldPattern `json:"vendor,omitempty" yaml:"vendor,omitempty"`
}

type BIOS struct {
	Vendor      FieldPattern `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Version     FieldPattern `json:"version,omitempty" yaml:"version,omitempty"`
	ReleaseDate FieldPattern `json:"releaseDate,omitempty" yaml:"releaseDate,omitempty"`
}

type Baseboard struct {
	Vendor       FieldPattern `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Product      FieldPattern `json:"product,omitempty" yaml:"product,omitempty"`
	Version      FieldPattern `json:"version,omitempty" yaml:"version,omitempty"`
	SerialNumber FieldPattern `json:"serialNumber,omitempty" yaml:"serialNumber,omitempty"`
}

type Product struct {
	Name         FieldPattern `json:"name,omitempty" yaml:"name,omitempty"`
	Vendor       FieldPattern `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	SerialNumber FieldPattern `json:"serialNumber,omitempty" yaml:"serialNumber,omitempty"`
}
