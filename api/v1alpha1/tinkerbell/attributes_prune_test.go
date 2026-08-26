package tinkerbell

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptrTo[T any](v T) *T { return &v }

func TestPruneEmptyNil(_ *testing.T) {
	var a *Attributes
	a.PruneEmpty() // must not panic
}

func TestPruneEmptyDropsEmptyPointerSubtrees(t *testing.T) {
	a := &Attributes{
		Chassis:   &Chassis{},
		BIOS:      &BIOS{},
		Baseboard: &Baseboard{},
		Product:   &Product{},
		CPU:       &CPU{},
		Memory:    &Memory{},
		BMC:       &BMC{},
	}
	a.PruneEmpty()
	if a.Chassis != nil || a.BIOS != nil || a.Baseboard != nil || a.Product != nil || a.CPU != nil || a.Memory != nil || a.BMC != nil {
		t.Errorf("empty pointer subtrees not dropped: %+v", a)
	}
}

func TestPruneEmptyPreservesPostCodeZero(t *testing.T) {
	a := &Attributes{
		// A device that reported nothing but a successful POST (code 0). The
		// zero PostCode must survive, and so must its parents.
		Product: &Product{Status: &ComponentStatus{PostCode: ptrTo(int32(0)), PostCodeStatus: "OK"}},
	}
	a.PruneEmpty()
	if a.Product == nil || a.Product.Status == nil || a.Product.Status.PostCode == nil {
		t.Fatalf("PostCode 0 chain was pruned: %+v", a.Product)
	}
	if *a.Product.Status.PostCode != 0 {
		t.Errorf("PostCode = %d, want 0", *a.Product.Status.PostCode)
	}
}

func TestPruneEmptyDropsEmptySliceElements(t *testing.T) {
	a := &Attributes{
		BlockDevices: []BlockDevice{{}, {Name: "sda"}, {}},
		NetworkInterfaces: []NetworkInterface{
			{Name: "tunl0", Ports: []NetworkPort{{}}}, // name-only; empty port must drop
			{Ports: []NetworkPort{{MAC: "aa:bb"}}},
			{}, // wholly empty; must drop
		},
	}
	a.PruneEmpty()

	if len(a.BlockDevices) != 1 || a.BlockDevices[0].Name != "sda" {
		t.Errorf("BlockDevices = %+v, want one entry Name=sda", a.BlockDevices)
	}
	if len(a.NetworkInterfaces) != 2 {
		t.Fatalf("NetworkInterfaces = %+v, want 2 (wholly-empty dropped)", a.NetworkInterfaces)
	}
	if a.NetworkInterfaces[0].Name != "tunl0" || len(a.NetworkInterfaces[0].Ports) != 0 {
		t.Errorf("NetworkInterfaces[0] = %+v, want name-only tunl0 with no ports", a.NetworkInterfaces[0])
	}
	if len(a.NetworkInterfaces[1].Ports) != 1 || a.NetworkInterfaces[1].Ports[0].MAC != "aa:bb" {
		t.Errorf("NetworkInterfaces[1].Ports = %+v, want one port MAC=aa:bb", a.NetworkInterfaces[1].Ports)
	}
}

func TestPruneEmptyKeepsProvenanceAndData(t *testing.T) {
	now := metav1.Now()
	a := &Attributes{
		LastUpdated:      &now,
		CollectionMethod: "redfish",
		BIOS:             &BIOS{Vendor: "Dell"},
	}
	a.PruneEmpty()
	if a.LastUpdated == nil || a.CollectionMethod != "redfish" {
		t.Errorf("provenance dropped: LastUpdated=%v CollectionMethod=%q", a.LastUpdated, a.CollectionMethod)
	}
	if a.BIOS == nil || a.BIOS.Vendor != "Dell" {
		t.Errorf("populated BIOS dropped: %+v", a.BIOS)
	}
}

func TestPruneEmptyCollapsesNestedEmpty(t *testing.T) {
	// Baseboard whose only field is an all-empty Status: the Status drops first,
	// leaving the Baseboard zero and thus dropped too (bottom-up).
	a := &Attributes{Baseboard: &Baseboard{Status: &ComponentStatus{}}}
	a.PruneEmpty()
	if a.Baseboard != nil {
		t.Errorf("Baseboard = %+v, want nil after its empty Status collapsed", a.Baseboard)
	}
	if !reflect.ValueOf(*a).IsZero() {
		t.Errorf("Attributes = %+v, want fully zero", a)
	}
}
