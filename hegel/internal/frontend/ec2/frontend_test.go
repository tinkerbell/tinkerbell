package ec2_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	. "github.com/tinkerbell/hegel/internal/frontend/ec2"
)

func init() {
	// Comment this out if you want to see Gin handler registration debug. Unforuntaely
	// Gin doesn't offer a way to do this per Engine instance instanatiation so we're forced to
	// use the package level function.
	gin.SetMode(gin.ReleaseMode)
}

func TestFrontendDynamicEndpoints(t *testing.T) {
	cases := []struct {
		Name     string
		Endpoint string
		Instance Instance
		Expect   string
	}{
		{
			Name:     "Userdata",
			Endpoint: "/2009-04-04/user-data",
			Instance: Instance{
				Userdata: "userdata",
			},
			Expect: "userdata",
		},
		{
			Name:     "InstanceID",
			Endpoint: "/2009-04-04/meta-data/instance-id",
			Instance: Instance{
				Metadata: Metadata{
					InstanceID: "instance-id",
				},
			},
			Expect: "instance-id",
		},
		{
			Name:     "Hostname",
			Endpoint: "/2009-04-04/meta-data/hostname",
			Instance: Instance{
				Metadata: Metadata{
					Hostname: "hostname",
				},
			},
			Expect: "hostname",
		},
		{
			Name:     "LocalHostname",
			Endpoint: "/2009-04-04/meta-data/local-hostname",
			Instance: Instance{
				Metadata: Metadata{
					LocalHostname: "local-hostname",
				},
			},
			Expect: "local-hostname",
		},
		{
			Name:     "IQN",
			Endpoint: "/2009-04-04/meta-data/iqn",
			Instance: Instance{
				Metadata: Metadata{
					IQN: "iqn",
				},
			},
			Expect: "iqn",
		},
		{
			Name:     "Plan",
			Endpoint: "/2009-04-04/meta-data/plan",
			Instance: Instance{
				Metadata: Metadata{
					Plan: "plan",
				},
			},
			Expect: "plan",
		},
		{
			Name:     "Facility",
			Endpoint: "/2009-04-04/meta-data/facility",
			Instance: Instance{
				Metadata: Metadata{
					Facility: "facility",
				},
			},
			Expect: "facility",
		},
		{
			Name:     "Tags",
			Endpoint: "/2009-04-04/meta-data/tags",
			Instance: Instance{
				Metadata: Metadata{
					Tags: []string{"tag1", "tag2"},
				},
			},
			Expect: "tag1\ntag2",
		},
		{
			Name:     "PublicKeys",
			Endpoint: "/2009-04-04/meta-data/public-keys",
			Instance: Instance{
				Metadata: Metadata{
					PublicKeys: []string{"key1", "key2"},
				},
			},
			Expect: "key1\nkey2",
		},
		{
			Name:     "PublicIPv4",
			Endpoint: "/2009-04-04/meta-data/public-ipv4",
			Instance: Instance{
				Metadata: Metadata{
					PublicIPv4: "public-ipv4",
				},
			},
			Expect: "public-ipv4",
		},
		{
			Name:     "PublicIPv6",
			Endpoint: "/2009-04-04/meta-data/public-ipv6",
			Instance: Instance{
				Metadata: Metadata{
					PublicIPv6: "public-ipv6",
				},
			},
			Expect: "public-ipv6",
		},
		{
			Name:     "LocalIPv4",
			Endpoint: "/2009-04-04/meta-data/local-ipv4",
			Instance: Instance{
				Metadata: Metadata{
					LocalIPv4: "local-ipv4",
				},
			},
			Expect: "local-ipv4",
		},
		{
			Name:     "OperatingSystemSlug",
			Endpoint: "/2009-04-04/meta-data/operating-system/slug",
			Instance: Instance{
				Metadata: Metadata{
					OperatingSystem: OperatingSystem{
						Slug: "slug",
					},
				},
			},
			Expect: "slug",
		},
		{
			Name:     "OperatingSystemDistro",
			Endpoint: "/2009-04-04/meta-data/operating-system/distro",
			Instance: Instance{
				Metadata: Metadata{
					OperatingSystem: OperatingSystem{
						Distro: "distro",
					},
				},
			},
			Expect: "distro",
		},
		{
			Name:     "OperatingSystemVersion",
			Endpoint: "/2009-04-04/meta-data/operating-system/version",
			Instance: Instance{
				Metadata: Metadata{
					OperatingSystem: OperatingSystem{
						Version: "version",
					},
				},
			},
			Expect: "version",
		},
		{
			Name:     "OperatingSystemImageTag",
			Endpoint: "/2009-04-04/meta-data/operating-system/image_tag",
			Instance: Instance{
				Metadata: Metadata{
					OperatingSystem: OperatingSystem{
						ImageTag: "image_tag",
					},
				},
			},
			Expect: "image_tag",
		},
		{
			Name:     "OperatingSystemLicenseActivationState",
			Endpoint: "/2009-04-04/meta-data/operating-system/license_activation/state",
			Instance: Instance{
				Metadata: Metadata{
					OperatingSystem: OperatingSystem{
						LicenseActivation: LicenseActivation{
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
			client := NewMockClient(ctrl)
			client.EXPECT().
				GetEC2Instance(gomock.Any(), gomock.Any()).
				Return(tc.Instance, nil).
				Times(2)

			router := gin.New()

			fe := New(client)
			fe.Configure(router)

			// Validate both with and without a trailing slash returns the same result.
			validate(t, router, tc.Endpoint, tc.Expect)
			validate(t, router, tc.Endpoint+"/", tc.Expect)
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
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockClient(ctrl)

			router := gin.New()

			fe := New(client)
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
	client := NewMockClient(ctrl)
	client.EXPECT().
		GetEC2Instance(gomock.Any(), gomock.Any()).
		Return(Instance{}, ErrInstanceNotFound)

	router := gin.New()

	fe := New(client)
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
	client := NewMockClient(ctrl)
	client.EXPECT().
		GetEC2Instance(gomock.Any(), gomock.Any()).
		Return(Instance{}, errors.New("generic error"))

	router := gin.New()

	fe := New(client)
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
		client := NewMockClient(ctrl)

		router := gin.New()

		fe := New(client)
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
