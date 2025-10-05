package nocloud_test

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/nocloud"
)

// ExampleClient demonstrates how to implement the nocloud.Client interface.
type ExampleClient struct {
	servers map[string]data.NoCloudInstance
}

func NewExampleClient() *ExampleClient {
	return &ExampleClient{
		servers: map[string]data.NoCloudInstance{
			"192.168.1.10": {
				Userdata: "#cloud-config\npackage_update: true\npackages:\n  - htop\n  - vim\n",
				Metadata: data.Metadata{
					InstanceID:    "server-001",
					LocalHostname: "web01.example.com",
				},
				NetworkConfig: map[string]interface{}{
					"version": 1,
					"config": []interface{}{
						map[string]interface{}{
							"type":        "physical",
							"name":        "eno1",
							"mac_address": "1c:34:da:12:34:56",
							"mtu":         1500,
						},
						map[string]interface{}{
							"type":        "physical",
							"name":        "eno2",
							"mac_address": "1c:34:da:12:34:57",
							"mtu":         1500,
						},
						map[string]interface{}{
							"type":            "bond",
							"name":            "bond0",
							"bond_interfaces": []string{"eno1", "eno2"},
							"params": map[string]interface{}{
								"bond-mode":             "802.3ad",
								"bond-miimon":           100,
								"bond-lacp_rate":        "fast",
								"bond-xmit_hash_policy": "layer3+4",
							},
							"subnets": []interface{}{
								map[string]interface{}{
									"type":            "static",
									"address":         "192.168.1.10/24",
									"gateway":         "192.168.1.1",
									"dns_nameservers": []string{"8.8.8.8", "8.8.4.4"},
								},
								map[string]interface{}{
									"type":            "static6",
									"address":         "2001:db8::10/64",
									"gateway":         "2001:db8::1",
									"dns_nameservers": []string{"2001:4860:4860::8888"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (c *ExampleClient) GetNoCloudInstance(_ context.Context, ip string) (data.NoCloudInstance, error) {
	if instance, exists := c.servers[ip]; exists {
		return instance, nil
	}
	return data.NoCloudInstance{}, nocloud.ErrInstanceNotFound
}

func Example() {
	// Create a client that can retrieve instance data
	client := NewExampleClient()

	// Create the NoCloud frontend
	frontend := nocloud.New(client)

	// Set up the router
	router := gin.New()
	frontend.Configure(router)

	// The router now has the following endpoints configured:
	// GET /meta-data        - Returns instance metadata in text/plain format
	// GET /user-data        - Returns cloud-config user data in text/plain format
	// GET /network-config   - Returns network configuration in text/yaml format

	// Start the server (in a real application)
	_ = &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	log.Println("NoCloud metadata API server starting on :8080")
	log.Println("Endpoints available:")
	log.Println("  GET /meta-data")
	log.Println("  GET /user-data")
	log.Println("  GET /network-config")

	// In a real application you would call:
	// server.ListenAndServe()

	fmt.Println("NoCloud metadata API configured successfully")
	// Output: NoCloud metadata API configured successfully
}
