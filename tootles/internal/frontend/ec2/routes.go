package ec2

import "github.com/tinkerbell/tinkerbell/data"

// TODO(chrisdoherty4) Figure out a better way to model routes; this approach is clunky and
// error prone. Ideally we have a way to define routes and retrieve the children of a route without
// manually defining everything.

type filterFunc func(i data.Ec2Instance) string

var dataRoutes = []struct {
	Endpoint string
	Filter   filterFunc
}{
	{
		Endpoint: "/user-data",
		Filter: func(i data.Ec2Instance) string {
			return i.Userdata
		},
	},
	{
		Endpoint: "/meta-data/instance-id",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.InstanceID
		},
	},
	{
		Endpoint: "/meta-data/hostname",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.Hostname
		},
	},
	{
		Endpoint: "/meta-data/local-hostname",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.LocalHostname
		},
	},
	{
		Endpoint: "/meta-data/iqn",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.IQN
		},
	},
	{
		Endpoint: "/meta-data/plan",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.Plan
		},
	},
	{
		Endpoint: "/meta-data/facility",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.Facility
		},
	},
	{
		Endpoint: "/meta-data/tags",
		Filter: func(i data.Ec2Instance) string {
			return join(i.Metadata.Tags)
		},
	},
	{
		Endpoint: "/meta-data/public-ipv4",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.PublicIPv4
		},
	},
	{
		Endpoint: "/meta-data/public-ipv6",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.PublicIPv6
		},
	},
	{
		Endpoint: "/meta-data/local-ipv4",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.LocalIPv4
		},
	},
	{
		Endpoint: "/meta-data/public-keys",
		Filter: func(i data.Ec2Instance) string {
			return join(i.Metadata.PublicKeys)
		},
	},
	{
		Endpoint: "/meta-data/operating-system/slug",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.OperatingSystem.Slug
		},
	},
	{
		Endpoint: "/meta-data/operating-system/distro",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.OperatingSystem.Distro
		},
	},
	{
		Endpoint: "/meta-data/operating-system/version",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.OperatingSystem.Version
		},
	},
	{
		Endpoint: "/meta-data/operating-system/image_tag",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.OperatingSystem.ImageTag
		},
	},
	{
		Endpoint: "/meta-data/operating-system/license_activation/state",
		Filter: func(i data.Ec2Instance) string {
			return i.Metadata.OperatingSystem.LicenseActivation.State
		},
	},
}
