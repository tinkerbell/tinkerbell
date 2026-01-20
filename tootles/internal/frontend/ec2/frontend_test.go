package ec2_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/ec2"
)

func init() {
	// Comment this out if you want to see Gin handler registration debug. Unforuntaely
	// Gin doesn't offer a way to do this per Engine instance instantiation so we're forced to
	// use the package level function.
	gin.SetMode(gin.ReleaseMode)
}

func TestFrontendDynamicEndpoints(t *testing.T) {
	cases := []struct {
		Name     string
		Endpoint string
		Instance data.Ec2Instance
		Expect   string
	}{
		{
			Name:     "Userdata",
			Endpoint: "/2009-04-04/user-data",
			Instance: data.Ec2Instance{
				Userdata: "userdata",
			},
			Expect: "userdata",
		},
		{
			Name:     "InstanceID",
			Endpoint: "/2009-04-04/meta-data/instance-id",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					InstanceID: "instance-id",
				},
			},
			Expect: "instance-id",
		},
		{
			Name:     "Hostname",
			Endpoint: "/2009-04-04/meta-data/hostname",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					Hostname: "hostname",
				},
			},
			Expect: "hostname",
		},
		{
			Name:     "LocalHostname",
			Endpoint: "/2009-04-04/meta-data/local-hostname",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					LocalHostname: "local-hostname",
				},
			},
			Expect: "local-hostname",
		},
		{
			Name:     "IQN",
			Endpoint: "/2009-04-04/meta-data/iqn",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					IQN: "iqn",
				},
			},
			Expect: "iqn",
		},
		{
			Name:     "Plan",
			Endpoint: "/2009-04-04/meta-data/plan",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					Plan: "plan",
				},
			},
			Expect: "plan",
		},
		{
			Name:     "Facility",
			Endpoint: "/2009-04-04/meta-data/facility",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					Facility: "facility",
				},
			},
			Expect: "facility",
		},
		{
			Name:     "Tags",
			Endpoint: "/2009-04-04/meta-data/tags",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					Tags: []string{"tag1", "tag2"},
				},
			},
			Expect: "tag1\ntag2",
		},
		{
			Name:     "PublicKeys",
			Endpoint: "/2009-04-04/meta-data/public-keys",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					PublicKeys: []string{"key1", "key2"},
				},
			},
			Expect: "key1\nkey2",
		},
		{
			Name:     "PublicIPv4",
			Endpoint: "/2009-04-04/meta-data/public-ipv4",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					PublicIPv4: "public-ipv4",
				},
			},
			Expect: "public-ipv4",
		},
		{
			Name:     "PublicIPv6",
			Endpoint: "/2009-04-04/meta-data/public-ipv6",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					PublicIPv6: "public-ipv6",
				},
			},
			Expect: "public-ipv6",
		},
		{
			Name:     "LocalIPv4",
			Endpoint: "/2009-04-04/meta-data/local-ipv4",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					LocalIPv4: "local-ipv4",
				},
			},
			Expect: "local-ipv4",
		},
		{
			Name:     "OperatingSystemSlug",
			Endpoint: "/2009-04-04/meta-data/operating-system/slug",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					OperatingSystem: data.OperatingSystem{
						Slug: "slug",
					},
				},
			},
			Expect: "slug",
		},
		{
			Name:     "OperatingSystemDistro",
			Endpoint: "/2009-04-04/meta-data/operating-system/distro",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					OperatingSystem: data.OperatingSystem{
						Distro: "distro",
					},
				},
			},
			Expect: "distro",
		},
		{
			Name:     "OperatingSystemVersion",
			Endpoint: "/2009-04-04/meta-data/operating-system/version",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					OperatingSystem: data.OperatingSystem{
						Version: "version",
					},
				},
			},
			Expect: "version",
		},
		{
			Name:     "OperatingSystemImageTag",
			Endpoint: "/2009-04-04/meta-data/operating-system/image_tag",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					OperatingSystem: data.OperatingSystem{
						ImageTag: "image_tag",
					},
				},
			},
			Expect: "image_tag",
		},
		{
			Name:     "OperatingSystemLicenseActivationState",
			Endpoint: "/2009-04-04/meta-data/operating-system/license_activation/state",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					OperatingSystem: data.OperatingSystem{
						LicenseActivation: data.LicenseActivation{
							State: "state",
						},
					},
				},
			},
			Expect: "state",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := ec2.NewMockClient(ctrl)
			client.EXPECT().
				GetEC2Instance(gomock.Any(), gomock.Any()).
				Return(tc.Instance, nil).
				Times(2)

			router := gin.New()

			fe := ec2.New(client, false)
			fe.Configure(router)

			// Validate both with and without a trailing slash returns the same result.
			validate(t, router, tc.Endpoint, tc.Expect)
			validate(t, router, tc.Endpoint+"/", tc.Expect)
		})
	}
}

func TestFrontendInstanceIDDynamicEndpoints(t *testing.T) {
	cases := []struct {
		Name     string
		Endpoint string
		Instance data.Ec2Instance
		Expect   string
	}{
		{
			Name:     "InstanceID",
			Endpoint: "/tootles/instanceID/instance-id-in-url/2009-04-04/meta-data/instance-id",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					InstanceID: "instance-id-in-url",
				},
			},
			Expect: "instance-id-in-url",
		},
		{
			Name:     "Hostname",
			Endpoint: "/tootles/instanceID/instance-id-in-url/2009-04-04/meta-data/hostname",
			Instance: data.Ec2Instance{
				Metadata: data.Metadata{
					InstanceID: "instance-id-in-url",
					Hostname:   "hostname",
				},
			},
			Expect: "hostname",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := ec2.NewMockClient(ctrl)
			client.EXPECT().
				GetEC2InstanceByInstanceID(gomock.Any(), gomock.Any()).
				Return(tc.Instance, nil).
				Times(2)

			router := gin.New()

			fe := ec2.New(client, true)
			fe.Configure(router)

			// Validate both with and without a trailing slash returns the same result.
			validate(t, router, tc.Endpoint, tc.Expect)
			validate(t, router, tc.Endpoint+"/", tc.Expect)
		})
	}
}

func TestFrontendInstanceIDEndpointsReturn404WhenDisabled(t *testing.T) {
	cases := []struct {
		Name     string
		Endpoint string
	}{
		{
			Name:     "InstanceIDDynamicEndpoint",
			Endpoint: "/tootles/instanceID/instance-id-in-url/2009-04-04/meta-data/instance-id",
		},
		{
			Name:     "HostnameDynamicEndpoint",
			Endpoint: "/tootles/instanceID/instance-id-in-url/2009-04-04/meta-data/hostname",
		},
		{
			Name:     "UserdataDynamicEndpoint",
			Endpoint: "/tootles/instanceID/instance-id-in-url/2009-04-04/user-data",
		},
		{
			Name:     "StaticEndpoint",
			Endpoint: "/tootles/instanceID/instance-id-in-url/2009-04-04/meta-data/operating-system/license_activation",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := ec2.NewMockClient(ctrl)
			// No expectations set on the client as the endpoints should not be registered

			router := gin.New()

			// Create frontend with instanceEndpoint flag set to false
			fe := ec2.New(client, false)
			fe.Configure(router)

			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", tc.Endpoint, nil)

			// RemoteAddr must be valid in case the request gets that far
			r.RemoteAddr = "10.10.10.10:0"

			router.ServeHTTP(w, r)

			if w.Code != http.StatusNotFound {
				t.Fatalf("Expected status: 404; Received status: %d for endpoint: %s", w.Code, tc.Endpoint)
			}
		})
	}
}

func TestFrontendStaticEndpoints(t *testing.T) {
	cases := []struct {
		Name     string
		Endpoint string
		Expect   string
	}{
		{
			Name:     "Root",
			Endpoint: "/2009-04-04",
			Expect: `meta-data/
user-data`,
		},
		{
			Name:     "Metadata",
			Endpoint: "/2009-04-04/meta-data",
			Expect: `facility
hostname
instance-id
iqn
local-hostname
local-ipv4
operating-system/
plan
public-ipv4
public-ipv6
public-keys
tags`,
		},
		{
			Name:     "MetadataOperatingSystem",
			Endpoint: "/2009-04-04/meta-data/operating-system",
			Expect: `distro
image_tag
license_activation/
slug
version`,
		},
		{
			Name:     "MetadataOperatingSystemLicenseActivation",
			Endpoint: "/2009-04-04/meta-data/operating-system/license_activation",
			Expect:   `state`,
		},
		{
			Name:     "MetadataOperatingSystemLicenseActivationViaInstanceEndpoint",
			Endpoint: "/tootles/instanceID/instance-id-in-url/2009-04-04/meta-data/operating-system/license_activation",
			Expect:   `state`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := ec2.NewMockClient(ctrl)

			router := gin.New()

			fe := ec2.New(client, true)
			fe.Configure(router)

			// Validate both with and without a trailing slash returns the same result.
			validate(t, router, tc.Endpoint, tc.Expect)
			validate(t, router, tc.Endpoint+"/", tc.Expect)
		})
	}
}

func validate(t *testing.T, router *gin.Engine, endpoint string, expect string) {
	t.Helper()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", endpoint, nil)

	// RemoteAddr must be valid for us to perform a lookup successfully. Because we're
	// mocking the client the address value doesn't matter.
	r.RemoteAddr = "10.10.10.10:0"

	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("\nEndpoint=%s\nExpected status: 200; Received status: %d; ", endpoint, w.Code)
	}

	if w.Body.String() != expect {
		t.Fatalf("\nExpected: %s;\nReceived: %s;\n(Endpoint=%s)", expect, w.Body.String(), endpoint)
	}
}

func Test404OnInstanceNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := ec2.NewMockClient(ctrl)
	client.EXPECT().
		GetEC2Instance(gomock.Any(), gomock.Any()).
		Return(data.Ec2Instance{}, ec2.ErrInstanceNotFound)

	router := gin.New()

	fe := ec2.New(client, true)
	fe.Configure(router)

	w := httptest.NewRecorder()
	// Ensure we're using an dynamic endpoint else we won't trigger an instance lookup.
	r := httptest.NewRequest("GET", "/2009-04-04/meta-data/hostname", nil)

	// RemoteAddr must be valid for us to perform a lookup successfully. Because we're
	// mocking the client the address value doesn't matter.
	r.RemoteAddr = "10.10.10.10:0"

	router.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected: 404; Received: %d", w.Code)
	}
}

func Test500OnGenericError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := ec2.NewMockClient(ctrl)
	client.EXPECT().
		GetEC2Instance(gomock.Any(), gomock.Any()).
		Return(data.Ec2Instance{}, errors.New("generic error"))

	router := gin.New()

	fe := ec2.New(client, true)
	fe.Configure(router)

	w := httptest.NewRecorder()
	// Ensure we're using an dynamic endpoint else we won't trigger an instance lookup.
	r := httptest.NewRequest("GET", "/2009-04-04/meta-data/hostname", nil)

	// RemoteAddr must be valid for us to perform a lookup successfully. Because we're
	// mocking the client the address value doesn't matter.
	r.RemoteAddr = "10.10.10.10:0"

	router.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("Expected: 500; Received: %d", w.Code)
	}
}

func Test400OnInvalidRemoteAddr(t *testing.T) {
	cases := []string{
		"invalid",
		"",
	}

	for _, invalidIP := range cases {
		ctrl := gomock.NewController(t)
		client := ec2.NewMockClient(ctrl)

		router := gin.New()

		fe := ec2.New(client, true)
		fe.Configure(router)

		w := httptest.NewRecorder()
		// Ensure we're using an dynamic endpoint else we won't trigger an instance lookup.
		r := httptest.NewRequest("GET", "/2009-04-04/meta-data/hostname", nil)

		// Invalidate the RemoteAddr of the request.
		r.RemoteAddr = invalidIP

		router.ServeHTTP(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("Expected: 400; Received: %d", w.Code)
		}
	}
}
