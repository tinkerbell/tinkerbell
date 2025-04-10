package kubernetes

import (
	"github.com/tinkerbell/tink/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// hardwareIPAddrIndex is the index used to retrieve hardware by IP address. It is used with
// the controller-runtimes MatchingFields selector.
const hardwareIPAddrIndex = ".Spec.Interfaces.DHCP.IP"

// hardwareIPIndexFunc satisfies the controller runtimes index.
func hardwareIPIndexFunc(obj client.Object) []string {
	hw, ok := obj.(*v1alpha1.Hardware)
	if !ok {
		return nil
	}
	resp := []string{}
	for _, iface := range hw.Spec.Interfaces {
		if iface.DHCP != nil && iface.DHCP.IP != nil && iface.DHCP.IP.Address != "" {
			resp = append(resp, iface.DHCP.IP.Address)
		}
	}
	return resp
}
