package flatfile_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/tinkerbell/hegel/internal/backend/flatfile"
	"github.com/tinkerbell/hegel/internal/frontend/ec2"
)

func TestGetEC2Instance(t *testing.T) {
	backend, err := FromYAMLFile("testdata/TestGetEC2Instance.yml")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		Name             string
		LookupIP         string
		ExpectedInstance *ec2.Instance
		ExpectedError    error
	}{
		{
			Name:     "IPFound",
			LookupIP: "10.10.10.10",
			ExpectedInstance: &ec2.Instance{
				Userdata: "test",
				Metadata: ec2.Metadata{
					InstanceID:    "instanceid",
					Hostname:      "hostname",
					LocalHostname: "localhostname",
					IQN:           "iqn",
					Plan:          "plan",
					Facility:      "facility",
					Tags:          []string{"foo", "bar"},
					OperatingSystem: ec2.OperatingSystem{
						Slug:     "slug",
						Distro:   "distro",
						Version:  "version",
						ImageTag: "imagetag",
						LicenseActivation: ec2.LicenseActivation{
							State: "licenseactivationstate",
						},
					},
					PublicIPv4: "10.10.10.10",
					PublicIPv6: "2001:db8:0:1:1:1:1:1",
					LocalIPv4:  "10.10.10.11",
				},
			},
		},
		{
			Name:          "IPNotFound",
			LookupIP:      "9.9.9.9",
			ExpectedError: ec2.ErrInstanceNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ec2Instance, err := backend.GetEC2Instance(context.Background(), tc.LookupIP)

			switch {
			case tc.ExpectedError != nil:
				if !errors.Is(err, tc.ExpectedError) {
					t.Fatalf("Expected: %v;\nReceived: %v", tc.ExpectedError, err)
				}

			case tc.ExpectedInstance != nil:
				if err != nil {
					t.Fatal(err)
				}

				if !cmp.Equal(&ec2Instance, tc.ExpectedInstance) {
					t.Error(cmp.Diff(ec2Instance, tc.ExpectedInstance))
				}
			}
		})
	}
}
