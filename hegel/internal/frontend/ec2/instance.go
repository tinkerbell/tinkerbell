package ec2

// Instance is a struct that contains the hardware data exposed from the EC2 API endpoints. For
// an explanation of the endpoints refer to the AWS EC2 Instance Metadata documentation.
//
//	https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-categories.html
//
// Note not all AWS EC2 Instance Metadata categories are supported as some are not applicable.
// Deviations from the AWS EC2 Instance Metadata should be documented here.
type Instance struct {
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
