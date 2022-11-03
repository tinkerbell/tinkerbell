package hardware

import "encoding/json"

// ExportedCacher is the structure in which hegel returns to clients using the old cacher data model
// exposes only certain fields of the hardware data returned by cacher.
type ExportedCacher struct {
	ID                                 string                   `json:"id"`
	Arch                               string                   `json:"arch"`
	State                              string                   `json:"state"`
	EFIBoot                            bool                     `json:"efi_boot"`
	Instance                           instance                 `json:"instance,omitempty"`
	PreinstalledOperatingSystemVersion interface{}              `json:"preinstalled_operating_system_version"`
	NetworkPorts                       []map[string]interface{} `json:"network_ports"`
	PlanSlug                           string                   `json:"plan_slug"`
	Facility                           string                   `json:"facility_code"`
	Hostname                           string                   `json:"hostname"`
	BondingMode                        int                      `json:"bonding_mode"`
}

type instance struct {
	ID       string `json:"id,omitempty"`
	State    string `json:"state,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	AllowPXE bool   `json:"allow_pxe,omitempty"`
	Rescue   bool   `json:"rescue,omitempty"`

	IPAddresses []map[string]interface{} `json:"ip_addresses,omitempty"`
	OS          *operatingSystem         `json:"operating_system_version,omitempty"`
	UserData    string                   `json:"userdata,omitempty"`

	CryptedRootPassword string `json:"crypted_root_password,omitempty"`

	StorageSource string   `json:"storage_source,omitempty"`
	Storage       *storage `json:"storage,omitempty"`
	SSHKeys       []string `json:"ssh_keys,omitempty"`
	NetworkReady  bool     `json:"network_ready,omitempty"`
	BootDriveHint string   `json:"boot_drive_hint,omitempty"`
}

type operatingSystem struct {
	Slug     string `json:"slug"`
	Distro   string `json:"distro"`
	Version  string `json:"version"`
	ImageTag string `json:"image_tag"`
	OsSlug   string `json:"os_slug"`
}

type disk struct {
	Device    string       `json:"device"`
	WipeTable bool         `json:"wipeTable,omitempty"`
	Paritions []*partition `json:"partitions,omitempty"`
}

type file struct {
	Path     string `json:"path"`
	Contents string `json:"contents,omitempty"`
	Mode     int    `json:"mode,omitempty"`
	UID      int    `json:"uid,omitempty"`
	GID      int    `json:"gid,omitempty"`
}

type filesystem struct {
	Mount struct {
		Device string             `json:"device"`
		Format string             `json:"format"`
		Files  []*file            `json:"files,omitempty"`
		Create *filesystemOptions `json:"create,omitempty"`
		Point  string             `json:"point"`
	} `json:"mount"`
}

type filesystemOptions struct {
	Force   bool     `json:"force,omitempty"`
	Options []string `json:"options,omitempty"`
}

type partition struct {
	Label    string      `json:"label"`
	Number   int         `json:"number"`
	Size     interface{} `json:"size"`
	Start    int         `json:"start,omitempty"`
	TypeGUID string      `json:"typeGuid,omitempty"`
}

type raid struct {
	Name    string   `json:"name"`
	Level   string   `json:"level"`
	Devices []string `json:"devices"`
	Spares  int      `json:"spares,omitempty"`
}

type storage struct {
	Disks       []*disk       `json:"disks,omitempty"`
	RAID        []*raid       `json:"raid,omitempty"`
	Filesystems []*filesystem `json:"filesystems,omitempty"`
}

// UnmarshalJSON implements the json.Unmarshaler interface for custom unmarshalling of ExportedCacher.
func (eh *ExportedCacher) UnmarshalJSON(b []byte) error {
	type ehj ExportedCacher
	var tmp ehj

	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}

	networkPorts := []map[string]interface{}{}
	for _, port := range tmp.NetworkPorts {
		if port["type"] == "data" {
			networkPorts = append(networkPorts, port)
		}
	}

	tmp.NetworkPorts = networkPorts
	*eh = ExportedCacher(tmp)
	return nil
}
