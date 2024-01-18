//go:build !integration

package kubernetes_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	. "github.com/tinkerbell/hegel/internal/backend/kubernetes"
	"github.com/tinkerbell/hegel/internal/frontend/ec2"
	tinkv1 "github.com/tinkerbell/tink/api/v1alpha1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetEC2Instance(t *testing.T) {
	cases := []struct {
		Name             string
		Hardware         tinkv1.Hardware
		ExpectedInstance ec2.Instance
	}{
		{
			Name: "AllFields",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Facility: &tinkv1.MetadataFacility{
							PlanSlug:        "plan-slug",
							PlanVersionSlug: "plan-version-slug",
							FacilityCode:    "facility-code",
						},
						Instance: &tinkv1.MetadataInstance{
							ID:       "instance-id",
							Hostname: "instance-hostname",
							Tags:     []string{"tag"},
							OperatingSystem: &tinkv1.MetadataInstanceOperatingSystem{
								Slug:     "slug",
								Distro:   "distro",
								Version:  "version",
								ImageTag: "image-tag",
							},
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "10.10.10.10",
									Family:  4,
									Public:  true,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					InstanceID:    "instance-id",
					Hostname:      "instance-hostname",
					LocalHostname: "instance-hostname",
					Plan:          "plan-slug",
					Facility:      "facility-code",
					Tags:          []string{"tag"},
					PublicIPv4:    "10.10.10.10",
					OperatingSystem: ec2.OperatingSystem{
						Slug:     "slug",
						Distro:   "distro",
						Version:  "version",
						ImageTag: "image-tag",
					},
				},
			},
		},
		{
			Name: "NilFacility",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Instance: &tinkv1.MetadataInstance{
							ID:       "instance-id",
							Hostname: "instance-hostname",
							Tags:     []string{"tag"},
							OperatingSystem: &tinkv1.MetadataInstanceOperatingSystem{
								Slug:     "slug",
								Distro:   "distro",
								Version:  "version",
								ImageTag: "image-tag",
							},
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "10.10.10.10",
									Family:  4,
									Public:  true,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					InstanceID:    "instance-id",
					Hostname:      "instance-hostname",
					LocalHostname: "instance-hostname",
					Tags:          []string{"tag"},
					PublicIPv4:    "10.10.10.10",
					OperatingSystem: ec2.OperatingSystem{
						Slug:     "slug",
						Distro:   "distro",
						Version:  "version",
						ImageTag: "image-tag",
					},
				},
			},
		},
		{
			Name: "NilInstance",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Facility: &tinkv1.MetadataFacility{
							PlanSlug:        "plan-slug",
							PlanVersionSlug: "plan-version-slug",
							FacilityCode:    "facility-code",
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					Plan:     "plan-slug",
					Facility: "facility-code",
				},
			},
		},
		{
			Name: "NilOperatingSystem",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Facility: &tinkv1.MetadataFacility{
							PlanSlug:        "plan-slug",
							PlanVersionSlug: "plan-version-slug",
							FacilityCode:    "facility-code",
						},
						Instance: &tinkv1.MetadataInstance{
							ID:       "instance-id",
							Hostname: "instance-hostname",
							Tags:     []string{"tag"},
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "10.10.10.10",
									Family:  4,
									Public:  true,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					InstanceID:    "instance-id",
					Hostname:      "instance-hostname",
					LocalHostname: "instance-hostname",
					Plan:          "plan-slug",
					Facility:      "facility-code",
					Tags:          []string{"tag"},
					PublicIPv4:    "10.10.10.10",
				},
			},
		},
		{
			Name: "MultiplePublicIPv4s",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Instance: &tinkv1.MetadataInstance{
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "10.10.10.10",
									Family:  4,
									Public:  true,
								},
								{
									Address: "172.15.0.1",
									Family:  4,
									Public:  true,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					PublicIPv4: "10.10.10.10",
				},
			},
		},
		{
			Name: "LocalIPv4s",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Instance: &tinkv1.MetadataInstance{
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "10.10.10.11",
									Family:  4,
									// Zero value is false but we want to be explicit.
									Public: false,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					LocalIPv4: "10.10.10.11",
				},
			},
		},
		{
			Name: "MultipleLocalIPv4s",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Instance: &tinkv1.MetadataInstance{
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "10.10.10.10",
									Family:  4,
									// Zero value is false but we want to be explicit.
									Public: false,
								},
								{
									Address: "172.15.0.1",
									Family:  4,
									// Zero value is false but we want to be explicit.
									Public: false,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					LocalIPv4: "10.10.10.10",
				},
			},
		},
		{
			Name: "PublicThenPrivateIPv4s",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Instance: &tinkv1.MetadataInstance{
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "10.10.10.10",
									Family:  4,
									Public:  true,
								},
								{
									Address: "172.15.0.1",
									Family:  4,
									// Zero value is false but we want to be explicit.
									Public: false,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					PublicIPv4: "10.10.10.10",
					LocalIPv4:  "172.15.0.1",
				},
			},
		},
		{
			Name: "PrivateThenPublicIPv4s",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Instance: &tinkv1.MetadataInstance{
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "10.10.10.10",
									Family:  4,
									// Zero value is false but we want to be explicit.
									Public: false,
								},
								{
									Address: "172.15.0.1",
									Family:  4,
									Public:  true,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					PublicIPv4: "172.15.0.1",
					LocalIPv4:  "10.10.10.10",
				},
			},
		},
		{
			Name: "PublicIPv6",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Instance: &tinkv1.MetadataInstance{
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "2001:db8:0:1:1:1:1:1",
									Family:  6,
									Public:  true,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					PublicIPv6: "2001:db8:0:1:1:1:1:1",
				},
			},
		},
		{
			Name: "MultipleIPv6s",
			Hardware: tinkv1.Hardware{
				Spec: tinkv1.HardwareSpec{
					Metadata: &tinkv1.HardwareMetadata{
						Instance: &tinkv1.MetadataInstance{
							Ips: []*tinkv1.MetadataInstanceIP{
								{
									Address: "2001:db8:0:1:1:1:1:1",
									Family:  6,
									Public:  true,
								},
								{
									Address: "1001:ca5:0:1:1:1:1:1",
									Family:  6,
									Public:  true,
								},
							},
						},
					},
				},
			},
			ExpectedInstance: ec2.Instance{
				Metadata: ec2.Metadata{
					PublicIPv6: "2001:db8:0:1:1:1:1:1",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			lister := NewMocklisterClient(ctrl)
			lister.EXPECT().
				List(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, l *tinkv1.HardwareList, _ ...crclient.ListOption) error {
					l.Items = append(l.Items, tc.Hardware)
					return nil
				})

			client := NewTestBackend(lister, nil)

			instance, err := client.GetEC2Instance(context.Background(), "10.10.10.10")
			if err != nil {
				t.Fatal(err)
			}

			if !cmp.Equal(instance, tc.ExpectedInstance) {
				t.Fatal(cmp.Diff(instance, tc.ExpectedInstance))
			}
		})
	}
}

func TestGetEC2InstanceWithClientError(t *testing.T) {
	expect := errors.New("foo-bar")
	ctrl := gomock.NewController(t)
	lister := NewMocklisterClient(ctrl)
	lister.EXPECT().
		List(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(expect)

	client := NewTestBackend(lister, nil)

	_, err := client.GetEC2Instance(context.Background(), "10.10.10.10")
	if !errors.Is(err, expect) {
		t.Fatal(err)
	}
}

func TestGetEC2InstanceWithGt1Result(t *testing.T) {
	ctrl := gomock.NewController(t)
	lister := NewMocklisterClient(ctrl)
	lister.EXPECT().
		List(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, l *tinkv1.HardwareList, _ ...crclient.ListOption) error {
			l.Items = make([]tinkv1.Hardware, 2)
			return nil
		})

	client := NewTestBackend(lister, nil)

	_, err := client.GetEC2Instance(context.Background(), "10.10.10.10")
	if err == nil {
		t.Fatal("Expected error for > 2 results")
	}
}

func TestGetEC2InstanceWithNoResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	lister := NewMocklisterClient(ctrl)
	lister.EXPECT().
		List(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	client := NewTestBackend(lister, nil)

	_, err := client.GetEC2Instance(context.Background(), "10.10.10.10")
	if !errors.Is(err, ec2.ErrInstanceNotFound) {
		t.Fatalf("Expected: ec2.ErrInstanceNotFound; Received: %v", err)
	}
}
