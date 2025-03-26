package attribute

import (
	"testing"

	"github.com/jaypipes/ghw"
	"github.com/jaypipes/ghw/pkg/block"
)

func TestXxx(t *testing.T) {

	cpu, err := ghw.CPU()
	if err != nil {
		t.Fatal(err)
	}
	cores := cpu.TotalCores
	threads := cpu.TotalHardwareThreads

	t.Logf("cores: %d, type: %[1]T", cores)
	t.Logf("threads: %d, type: %[1]T", threads)
	t.Log(len(cpu.Processors))
	for _, p := range cpu.Processors {
		t.Logf("total cores: %v, type: %[1]T", p.TotalCores)
		t.Logf("total threads: %v, type: %[1]T", p.TotalHardwareThreads)
		t.Logf("vendor: %v, type: %[1]T", p.Vendor)
		t.Logf("model: %v, type: %[1]T", p.Model)
		for _, c := range p.Capabilities {
			t.Logf("capability: %v, type: %[1]T", c)
		}
	}

	memory, err := ghw.Memory()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("total memory: %v, type: %[1]T", toGB(memory.TotalPhysicalBytes))
	t.Logf("total memory: %v, type: %[1]T", toGB(memory.TotalUsableBytes))

	b, err := ghw.Block()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range b.Disks {
		if d.StorageController != block.StorageControllerLoop {
			t.Logf("name: %v, type: %[1]T", d.Name)
			t.Logf("controller: %v, type: %[1]T", d.StorageController)
			t.Logf("drive type: %v, type: %[1]T", d.DriveType)
			t.Logf("size: %v, type: %[1]T", toGB(d.SizeBytes))
			t.Logf("block size: %v, type: %[1]T", d.PhysicalBlockSizeBytes)
			t.Logf("vendor: %v, type: %[1]T", d.Vendor)
			t.Logf("model: %v, type: %[1]T", d.Model)
		}
	}

	net, err := ghw.Network()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range net.NICs {
		if !n.IsVirtual {
			t.Logf("name: %v, type: %[1]T", n.Name)
			t.Logf("mac address: %v, type: %[1]T", n.MACAddress)
			//t.Logf("capabilities: %v, type: %[1]T", n.Capabilities)
			t.Logf("speed: %v, type: %[1]T", n.Speed)
			for _, c := range n.Capabilities {
				if c.IsEnabled {
					t.Logf("capability: %v, type: %[1]T", c.Name)
				}
			}
		}
	}

	pci, err := ghw.PCI()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range pci.Devices {
		t.Logf("address: %v, type: %[1]T", d.Address)
		t.Logf("vendor: %v, type: %[1]T", d.Vendor.Name)
		t.Logf("product: %v, type: %[1]T", d.Product.Name)
		t.Logf("class: %v, type: %[1]T", d.Class.Name)
		t.Logf("subsystem: %v, type: %[1]T", d.Subsystem.Name)
		t.Logf("driver: %v, type: %[1]T", d.Driver)
	}
	t.Log(len(pci.Devices))

	gpu, err := ghw.GPU()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("GPU")
	for _, d := range gpu.GraphicsCards {
		t.Logf("address: %v, type: %[1]T", d.Address)
		t.Logf("vendor: %v, type: %[1]T", d.DeviceInfo.Vendor.Name)
		t.Logf("product: %v, type: %[1]T", d.DeviceInfo.Product.Name)
		t.Logf("class: %v, type: %[1]T", d.DeviceInfo.Class.Name)
		t.Logf("subsystem: %v, type: %[1]T", d.DeviceInfo.Subsystem.Name)
		t.Logf("driver: %v, type: %[1]T", d.DeviceInfo.Driver)
	}

	t.Log("Chassis")
	chassis, err := ghw.Chassis()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("asset tag: %v, type: %[1]T", chassis.AssetTag)
	t.Logf("serial number: %v, type: %[1]T", chassis.SerialNumber)
	t.Logf("type: %v, type: %[1]T", chassis.Type)
	t.Logf("type description: %v, type: %[1]T", chassis.TypeDescription)
	t.Logf("vendor: %v, type: %[1]T", chassis.Vendor)
	t.Logf("version: %v, type: %[1]T", chassis.Version)

	t.Log("BIOS")
	bios, err := ghw.BIOS()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("vendor: %v, type: %[1]T", bios.Vendor)
	t.Logf("version: %v, type: %[1]T", bios.Version)
	t.Logf("release date: %v, type: %[1]T", bios.Date)

	t.Log("Baseboard")
	baseboard, err := ghw.Baseboard()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("vendor: %v, type: %[1]T", baseboard.Vendor)
	t.Logf("model: %v, type: %[1]T", baseboard.Product)
	t.Logf("version: %v, type: %[1]T", baseboard.Version)
	t.Logf("serial number: %v, type: %[1]T", baseboard.SerialNumber)

	t.Log("product")
	product, err := ghw.Product()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("vendor: %v, type: %[1]T", product.Vendor)
	t.Logf("name: %v, type: %[1]T", product.Name)
	t.Logf("serial number: %v, type: %[1]T", product.SerialNumber)
	t.Logf("uuid: %v, type: %[1]T", product.UUID)
	t.Logf("family: %v, type: %[1]T", product.Family)
	t.Logf("version: %v, type: %[1]T", product.Version)

	t.Fail()
}
