package job

import (
	"context"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/packethost/pkg/log"
	"github.com/tinkerbell/boots/client"
	"github.com/tinkerbell/boots/client/cacher"
	"go.uber.org/zap/zaptest"
)

type Mock Job

// NewMock returns a mock Job with only minimal fields set, it is useful only for tests
func NewMock(t zaptest.TestingT, slug, facility string) Mock {
	slugs := strings.Split(slug, ":")
	slug = slugs[0]
	var planVersion string
	if len(slugs) > 1 {
		planVersion = slugs[1]
	}

	arch := "x86_64"
	if strings.Contains(slug, ".arm") || strings.Contains(slug, "baremetal_2a") || strings.Contains(slug, "baremetal_hua") {
		arch = "aarch64"
	}

	uefi := false
	if arch == "aarch64" || slug == "c2.medium.x86" {
		uefi = true
	}

	servicesVersion := client.ServicesVersion{}
	if strings.Contains(slug, "custom-osie") {
		servicesVersion.OSIE = "osie-v18.08.13.00"
	}

	mockLog := log.Test(t, "job.Mock")

	return Mock{
		Logger: mockLog.With("mock", true, "slug", slug, "arch", arch, "uefi", uefi),
		hardware: &cacher.HardwareCacher{
			ID:              uuid.New().String(),
			PlanSlug:        slug,
			PlanVersionSlug: planVersion,
			FacilityCode:    facility,
			Arch:            arch,
			State:           "provisionable",
			UEFI:            uefi,
			ServicesVersion: servicesVersion,
		},
		instance: &client.Instance{
			State: "provisioning",
		},
	}
}

func NewMockFromDiscovery(d client.Discoverer, mac net.HardwareAddr) Mock {
	mockLog, _ := log.Init("job.Mock")
	j := Job{Logger: mockLog, mac: mac}
	j.setup(context.Background(), d)

	return Mock(j)
}

func (m Mock) Job() Job {
	return Job(m)
}

func (m *Mock) DropInstance() {
	m.instance = nil
}

func (m *Mock) SetIP(ip net.IP) {
	m.ip = ip
}

func (m *Mock) SetIPXEScriptURL(url string) {
	m.instance.IPXEScriptURL = url
}

func (m *Mock) SetUserData(userdata string) {
	m.instance.UserData = userdata
}

func (m *Mock) SetMAC(mac string) {
	_m, err := net.ParseMAC(mac)
	if err != nil {
		panic(err)
	}
	m.mac = _m
}

func (m *Mock) SetManufacturer(slug string) {
	hp := m.hardware
	h := hp.(*cacher.HardwareCacher)
	h.Manufacturer = client.Manufacturer{Slug: slug}
}

func (m *Mock) SetOSDistro(distro string) {
	m.hardware.OperatingSystem().Distro = distro
}

func (m *Mock) SetOSSlug(slug string) {
	m.hardware.OperatingSystem().Slug = slug
	m.hardware.OperatingSystem().OsSlug = slug
}

func (m *Mock) SetOSVersion(version string) {
	m.hardware.OperatingSystem().Version = version
}

func (m *Mock) SetOSImageTag(tag string) {
	m.hardware.OperatingSystem().ImageTag = tag
}

func (m *Mock) SetOSInstaller(installer string) {
	m.hardware.OperatingSystem().Installer = installer
}

func (m *Mock) SetOSInstallerData(installerData *client.InstallerData) {
	m.hardware.OperatingSystem().InstallerData = installerData
}

func (m *Mock) SetPassword(password string) {
	m.instance.CryptedRootPassword = "insecure"
	m.instance.PasswordHash = "insecure"
}

func (m *Mock) SetState(state string) {
	hp := m.hardware
	h := hp.(*cacher.HardwareCacher)
	h.State = client.HardwareState(state)
}

func (m *Mock) SetBootDriveHint(drive string) {
	m.instance.BootDriveHint = drive
}

func (m *Mock) SetRescue(b bool) {
	i := m.instance
	i.Rescue = b
}

func MakeHardwareWithInstance() (*cacher.DiscoveryCacher, []client.MACAddr, string) {
	macIPMI := client.MACAddr([6]byte{0x00, 0xDE, 0xAD, 0xBE, 0xEF, 0x00})
	mac0 := client.MACAddr([6]byte{0x00, 0xBA, 0xDD, 0xBE, 0xEF, 0x00})
	mac1 := client.MACAddr([6]byte{0x00, 0xBA, 0xDD, 0xBE, 0xEF, 0x01})
	mac2 := client.MACAddr([6]byte{0x00, 0xBA, 0xDD, 0xBE, 0xEF, 0x02})
	mac3 := client.MACAddr([6]byte{0x00, 0xBA, 0xDD, 0xBE, 0xEF, 0x03})

	instanceId := uuid.New().String()
	d := &cacher.DiscoveryCacher{
		HardwareCacher: &cacher.HardwareCacher{
			ID:   uuid.New().String(),
			Name: "TestSetupInstanceHardwareName",
			NetworkPorts: []client.Port{
				client.Port{
					Type: "data",
					Name: "eth0",
					Data: struct {
						MAC  *client.MACAddr `json:"mac"`
						Bond string          `json:"bond"`
					}{
						MAC:  &mac0,
						Bond: "bond0",
					},
				},
				client.Port{
					Type: "data",
					Name: "eth1",
					Data: struct {
						MAC  *client.MACAddr `json:"mac"`
						Bond string          `json:"bond"`
					}{
						MAC:  &mac1,
						Bond: "bond0",
					},
				},
				client.Port{
					Type: "data",
					Name: "eth2",
					Data: struct {
						MAC  *client.MACAddr `json:"mac"`
						Bond string          `json:"bond"`
					}{
						MAC:  &mac2,
						Bond: "bond1",
					},
				},
				client.Port{
					Type: "data",
					Name: "eth3",
					Data: struct {
						MAC  *client.MACAddr `json:"mac"`
						Bond string          `json:"bond"`
					}{
						MAC:  &mac3,
						Bond: "bond1",
					},
				},
				client.Port{
					Type: "ipmi",
					Name: "ipmi0",
					Data: struct {
						MAC  *client.MACAddr `json:"mac"`
						Bond string          `json:"bond"`
					}{
						MAC: &macIPMI,
					},
				},
			},
			Instance: &client.Instance{
				ID:       instanceId,
				Hostname: "TestSetupInstanceHostname",
				IPs: []client.IP{
					client.IP{
						Address:    net.ParseIP("192.168.100.2"),
						Gateway:    net.ParseIP("192.168.100.1"),
						Netmask:    net.ParseIP("192.168.100.255"),
						Family:     4,
						Management: true,
						Public:     true,
					},
					client.IP{
						Address:    net.ParseIP("192.168.200.2"),
						Gateway:    net.ParseIP("192.168.200.1"),
						Netmask:    net.ParseIP("192.168.200.255"),
						Family:     4,
						Management: true,
						Public:     false,
					},
				},
			},
			IPMI: client.IP{
				Address:    net.ParseIP("192.168.0.2"),
				Gateway:    net.ParseIP("192.168.0.1"),
				Netmask:    net.ParseIP("192.168.0.255"),
				Family:     4,
				Management: true,
				Public:     false,
			},
		},
	}

	return d, []client.MACAddr{macIPMI, mac0, mac1, mac2, mac3}, instanceId
}

func MakeHardwareWithoutInstance() (*cacher.DiscoveryCacher, client.MACAddr) {
	mac := client.MACAddr([6]byte{0x00, 0xBA, 0xDD, 0xBE, 0xEF, 0x00})
	d := &cacher.DiscoveryCacher{
		HardwareCacher: &cacher.HardwareCacher{
			ID:   uuid.New().String(),
			Name: "TestSetupWithoutInstanceHardwareName",
			NetworkPorts: []client.Port{
				client.Port{
					Type: "data",
					Name: "eth0",
					Data: struct {
						MAC  *client.MACAddr `json:"mac"`
						Bond string          `json:"bond"`
					}{
						MAC:  &mac,
						Bond: "bond0",
					},
				},
			},
		},
	}

	return d, mac
}
