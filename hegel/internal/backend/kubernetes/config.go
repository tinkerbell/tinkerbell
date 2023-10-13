package kubernetes

import (
	"k8s.io/client-go/rest"
)

// Config used by the NewBackend function family.
type Config struct {
	// Kubeconfig is a path to a valid kubeconfig file. When in-cluster defaults to the in-cluster
	// config. Optional.
	Kubeconfig string

	// APIServerAddress is the address of the kubernetes cluster (https://hostname:port). Optional.
	APIServerAddress string

	// Namespace restricts the scope of the backend such that Hardware objects are retrieved from
	// this namespace only. Optional.
	Namespace string

	// ClientConfig is a Kubernetes client config. If specified, it will be used instead of
	// constructing a client using the other configuration in this object. Optional.
	ClientConfig *rest.Config
}
