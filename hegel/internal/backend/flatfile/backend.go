package flatfile

import (
	"context"

	"github.com/tinkerbell/hegel/internal/frontend/ec2"
)

// Backend is a file-based implementation of a backend. It's primary use-case is testing.
type Backend struct {
	// Map of IPv4 addresses to instances.
	instances map[string]Instance
}

// New returns a new instance of Backend.
func NewBackend(instances []Instance) *Backend {
	return &Backend{instances: toIPInstanceMap(instances)}
}

// RetrieveEC2InstanceByIP satisfies ec2.Client.
func (b *Backend) GetEC2Instance(_ context.Context, ip string) (ec2.Instance, error) {
	hw, ok := b.instances[ip]
	if !ok {
		return ec2.Instance{}, ec2.ErrInstanceNotFound
	}

	return toEC2Instance(hw), nil
}

// IsHealthy satisfies healthcheck.Client.
func (b *Backend) IsHealthy(context.Context) bool {
	return true
}

func toEC2Instance(i Instance) ec2.Instance {
	return ec2.Instance{
		Userdata: i.Userdata,
		Metadata: ec2.Metadata{
			InstanceID:    i.Metadata.ID,
			Hostname:      i.Metadata.Hostname,
			LocalHostname: i.Metadata.LocalHostname,
			IQN:           i.Metadata.IQN,
			Plan:          i.Metadata.Plan,
			Facility:      i.Metadata.Facility,
			Tags:          i.Metadata.Tags,
			OperatingSystem: ec2.OperatingSystem{
				Slug:     i.Metadata.OS.Slug,
				Distro:   i.Metadata.OS.Distro,
				Version:  i.Metadata.OS.Version,
				ImageTag: i.Metadata.OS.ImageTag,
				LicenseActivation: ec2.LicenseActivation{
					State: i.Metadata.OS.LicenseActivationState,
				},
			},
			PublicIPv4: i.Metadata.IPv4.Public,
			PublicIPv6: i.Metadata.IPv6.Public,
			LocalIPv4:  i.Metadata.IPv4.Local,
		},
	}
}

// Instance is a representation of a machine instance.
type Instance struct {
	Userdata string `yaml:"userdata"`
	Metadata struct {
		ID            string   `yaml:"id"`
		Hostname      string   `yaml:"hostname"`
		LocalHostname string   `yaml:"localHostname"`
		IQN           string   `yaml:"iqn"`
		Plan          string   `yaml:"plan"`
		Facility      string   `yaml:"facility"`
		Tags          []string `yaml:"tags"`
		IPv4          struct {
			Local  string `yaml:"local"`
			Public string `yaml:"public"`
		} `yaml:"ipv4"`
		IPv6 struct {
			Public string `yaml:"public"`
		} `yaml:"ipv6"`
		OS struct {
			Slug                   string `yaml:"slug"`
			Distro                 string `yaml:"distro"`
			Version                string `yaml:"version"`
			ImageTag               string `yaml:"imageTag"`
			LicenseActivationState string `yaml:"licenseActivationState"`
		} `yaml:"os"`
	} `yaml:"metadata"`
}

func toIPInstanceMap(instances []Instance) map[string]Instance {
	m := make(map[string]Instance, len(instances))
	for _, i := range instances {
		m[i.Metadata.IPv4.Public] = i
	}
	return m
}
