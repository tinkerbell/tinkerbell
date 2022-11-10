package kubernetes

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Config used by the NewBackend function family.
type Config struct {
	*rest.Config
	Namespace string
}

// NewConfig loads the kubeconfig overriding it with kubeAPI.
func NewConfig(kubeconfig, kubeAPI, namespace string) (Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfig

	overrides := &clientcmd.ConfigOverrides{
		ClusterInfo: clientcmdapi.Cluster{
			Server: kubeAPI,
		},
		Context: clientcmdapi.Context{
			Namespace: namespace,
		},
	}

	kubeBackendCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	config, err := kubeBackendCfg.ClientConfig()
	if err != nil {
		return Config{}, err
	}

	namespace, _, err = kubeBackendCfg.Namespace()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Config:    config,
		Namespace: namespace,
	}, nil
}
