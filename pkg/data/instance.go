package data

// Ec2Instance is a struct that contains the hardware data exposed from the EC2 API endpoints. For
// an explanation of the endpoints refer to the AWS EC2 Ec2Instance Metadata documentation.
//
//	https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-categories.html
//
// Note not all AWS EC2 Ec2Instance Metadata categories are supported as some are not applicable.
// Deviations from the AWS EC2 Ec2Instance Metadata should be documented here.
type Ec2Instance struct {
	Userdata string
	Metadata Metadata
}

// Metadata is a part of Instance.
type Metadata struct {
	InstanceID      string
	Hostname        string
	LocalHostname   string
	IQN             string
	Plan            string
	Facility        string
	Tags            []string
	PublicKeys      []string
	PublicIPv4      string
	PublicIPv6      string
	LocalIPv4       string
	OperatingSystem OperatingSystem
}

// OperatingSystem is part of Metadata.
type OperatingSystem struct {
	Slug              string
	Distro            string
	Version           string
	ImageTag          string
	LicenseActivation LicenseActivation
}

// LicenseActivation is part of OperatingSystem.
type LicenseActivation struct {
	State string
}

// Instance is a representation of the instance metadata. Its based on the rooitio hub action
// and should have just enough information for it to work.
type HackInstance struct {
	Metadata struct {
		Instance struct {
			Storage struct {
				Disks []struct {
					Device     string `json:"device"`
					Partitions []struct {
						Label  string `json:"label"`
						Number int    `json:"number"`
						Size   uint64 `json:"size"`
					} `json:"partitions"`
					WipeTable bool `json:"wipe_table"`
				} `json:"disks"`
				Filesystems []struct {
					Mount struct {
						Create struct {
							Options []string `json:"options"`
						} `json:"create"`
						Device string `json:"device"`
						Format string `json:"format"`
						Point  string `json:"point"`
					} `json:"mount"`
				} `json:"filesystems"`
			} `json:"storage"`
		} `json:"instance"`
	} `json:"metadata"`
}
