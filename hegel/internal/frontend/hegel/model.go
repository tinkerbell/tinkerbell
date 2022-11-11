//go:build ignore

package hegel

import (
	"encoding/json"

	tinkv1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
)

/*

These models are here to ensure this package compiles. They'll be deleted once its converted
to a proper frontend

*/

// K8sHardware satisfies the Export() requirements of the EC2 Metadata filter handling.
type K8sHardware struct {
	Hardware *tinkv1.Hardware    `json:"-"`
	Metadata K8sHardwareMetadata `json:"metadata,omitempty"`
}

// Exprot returns a JSON representation of h.
func (h K8sHardware) Export() ([]byte, error) {
	return json.Marshal(h)
}

// ID retrieves the instance ID.
func (h K8sHardware) ID() (string, error) {
	return h.Metadata.Instance.ID, nil
}

type K8sHardwareMetadata struct {
	Userdata   *string                     `json:"userdata,omitempty"`
	Vendordata *string                     `json:"vendordata,omitempty"`
	Instance   K8sHardwareMetadataInstance `json:"instance,omitempty"`
	//+optional
	Interfaces []K8sNetworkInterface `json:"interfaces,omitempty"`
	Gateway    string                `json:"gateway,omitempty"`
}

type K8sNetworkInterface struct {
	//+optional
	MAC     string `json:"mac,omitempty"`
	Address string `json:"address,omitempty"`
	Netmask string `json:"netmask,omitempty"`
	Family  int64  `json:"family,omitempty"`
}

type K8sHardwareMetadataInstance struct {
	Disks           []K8sHardwareDisk                          `json:"disks,omitempty"`
	ID              string                                     `json:"id,omitempty"`
	Hostname        string                                     `json:"hostname,omitempty"`
	Plan            string                                     `json:"plan,omitempty"`
	Factility       string                                     `json:"factility,omitempty"`
	Tags            []string                                   `json:"tags,omitempty"`
	SSHKeys         []string                                   `json:"ssh_keys,omitempty"`
	OperatingSystem K8sHardwareMetadataInstanceOperatingSystem `json:"operating_system,omitempty"`
	Network         K8sHardwareMetadataInstanceNetwork         `json:"network,omitempty"`
}

type K8sHardwareMetadataInstanceOperatingSystem struct {
	Slug     string `json:"slug,omitempty"`
	Distro   string `json:"distro,omitempty"`
	Version  string `json:"version,omitempty"`
	ImageTag string `json:"image_tag,omitempty"`
}

type K8sHardwareMetadataInstanceNetwork struct {
	Addresses []K8sHardwareMetadataInstanceNetworkAddress `json:"addresses,omitempty"`
}

type K8sHardwareMetadataInstanceNetworkAddress struct {
	AddressFamily int64  `json:"address_family,omitempty"`
	Address       string `json:"address,omitempty"`
	Public        bool   `json:"public,omitempty"`
}

type K8sHardwareDisk struct {
	//+optional
	Device string `json:"device,omitempty"`
}
