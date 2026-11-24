package crd

import (
	"regexp"
	"testing"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// TestMachineProviderNamesMatchBmclib checks that the Machine CRD accepts, in providers and
// preferredOrder, every provider name bmclib registers, so a bmclib bump that adds a provider
// fails here instead of leaving Machines unable to name it.
func TestMachineProviderNamesMatchBmclib(t *testing.T) {
	var machine v1.CustomResourceDefinition
	if err := yaml.Unmarshal(TinkerbellDefaults["machines.bmc.tinkerbell.org"], &machine); err != nil {
		t.Fatal(err)
	}
	providerOptions := machine.Spec.Versions[0].Schema.OpenAPIV3Schema.
		Properties["spec"].Properties["connection"].Properties["providerOptions"]

	for _, field := range []string{"providers", "preferredOrder"} {
		items := providerOptions.Properties[field].Items
		if items == nil || items.Schema == nil || items.Schema.Pattern == "" {
			t.Fatalf("providerOptions.%s has no item pattern", field)
		}
		pattern := regexp.MustCompile(items.Schema.Pattern)
		for _, name := range bmclib.ProviderNames() {
			if !pattern.MatchString(name) {
				t.Errorf("providerOptions.%s doesn't accept bmclib provider %q: pattern %s", field, name, items.Schema.Pattern)
			}
		}
	}
}
