package backend

import (
"context"
"errors"
"fmt"
"net/http"
"testing"

"github.com/google/go-cmp/cmp"
v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
"github.com/tinkerbell/tinkerbell/pkg/data"
"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/ec2"
metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockReader struct {
	hw  *v1alpha1.Hardware
	err error
}

func (m *mockReader) ReadHardware(_ context.Context, _, _ string, _ data.ReadListOptions) (*v1alpha1.Hardware, error) {
	return m.hw, m.err
}

// notFoundError satisfies the notFounder interface and the apierrors.APIStatus interface.
type notFoundError struct {
	msg string
}

func (e notFoundError) Error() string  { return e.msg }
func (e notFoundError) NotFound() bool { return true }
func (e notFoundError) Status() metav1.Status {
	return metav1.Status{
		Reason: metav1.StatusReasonNotFound,
		Code:   http.StatusNotFound,
	}
}

func TestGetEC2Instance(t *testing.T) {
	userData := "my-user-data"
	tests := map[string]struct {
		reader  *mockReader
		ip      string
		want    data.Ec2Instance
		wantErr error
	}{
		"success with full metadata": {
			reader: &mockReader{
				hw: &v1alpha1.Hardware{
					Spec: v1alpha1.HardwareSpec{
						UserData: &userData,
						Metadata: &v1alpha1.HardwareMetadata{
							Instance: &v1alpha1.MetadataInstance{
								ID:       "inst-123",
								Hostname: "my-host",
								Tags:     []string{"tag1", "tag2"},
								OperatingSystem: &v1alpha1.MetadataInstanceOperatingSystem{
									Slug:     "ubuntu",
									Distro:   "ubuntu",
									Version:  "20.04",
									ImageTag: "v1",
								},
								Ips: []*v1alpha1.MetadataInstanceIP{
									{Address: "1.2.3.4", Family: 4, Public: true},
									{Address: "10.0.0.1", Family: 4, Public: false},
									{Address: "2001:db8::1", Family: 6, Public: true},
								},
							},
							Facility: &v1alpha1.MetadataFacility{
								PlanSlug:     "c3.small.x86",
								FacilityCode: "sjc1",
							},
						},
					},
				},
			},
			ip: "10.0.0.1",
			want: data.Ec2Instance{
				Userdata: "my-user-data",
				Metadata: data.Metadata{
					InstanceID:    "inst-123",
					Hostname:      "my-host",
					LocalHostname: "my-host",
					Tags:          []string{"tag1", "tag2"},
					PublicIPv4:    "1.2.3.4",
					LocalIPv4:     "10.0.0.1",
					PublicIPv6:    "2001:db8::1",
					Plan:          "c3.small.x86",
					Facility:      "sjc1",
					OperatingSystem: data.OperatingSystem{
						Slug:     "ubuntu",
						Distro:   "ubuntu",
						Version:  "20.04",
						ImageTag: "v1",
					},
				},
			},
		},
		"success with nil metadata": {
			reader: &mockReader{
				hw: &v1alpha1.Hardware{},
			},
			ip:   "10.0.0.1",
			want: data.Ec2Instance{},
		},
		"not found error wraps as ec2.ErrInstanceNotFound": {
			reader: &mockReader{
				err: notFoundError{msg: "hardware not found: 10.0.0.1"},
			},
			ip:      "10.0.0.1",
			wantErr: ec2.ErrInstanceNotFound,
		},
		"generic error returned as-is": {
			reader: &mockReader{
				err: fmt.Errorf("connection refused"),
			},
			ip:      "10.0.0.1",
			wantErr: fmt.Errorf("connection refused"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
b := New(tt.reader)
got, err := b.GetEC2Instance(context.Background(), tt.ip)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if errors.Is(tt.wantErr, ec2.ErrInstanceNotFound) {
					if !errors.Is(err, ec2.ErrInstanceNotFound) {
						t.Fatalf("expected error wrapping ec2.ErrInstanceNotFound, got: %v", err)
					}
					return
				}
				if err.Error() != tt.wantErr.Error() {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("ec2 instance mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetEC2InstanceByInstanceID(t *testing.T) {
	tests := map[string]struct {
		reader  *mockReader
		want    data.Ec2Instance
		wantErr error
	}{
		"success": {
			reader: &mockReader{
				hw: &v1alpha1.Hardware{
					Spec: v1alpha1.HardwareSpec{
						Metadata: &v1alpha1.HardwareMetadata{
							Instance: &v1alpha1.MetadataInstance{
								ID:       "inst-456",
								Hostname: "host-456",
							},
						},
					},
				},
			},
			want: data.Ec2Instance{
				Metadata: data.Metadata{
					InstanceID:    "inst-456",
					Hostname:      "host-456",
					LocalHostname: "host-456",
				},
			},
		},
		"not found": {
			reader:  &mockReader{err: notFoundError{msg: "not found"}},
			wantErr: ec2.ErrInstanceNotFound,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
b := New(tt.reader)
got, err := b.GetEC2InstanceByInstanceID(context.Background(), "inst-456")

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if errors.Is(tt.wantErr, ec2.ErrInstanceNotFound) && !errors.Is(err, ec2.ErrInstanceNotFound) {
					t.Fatalf("expected ec2.ErrInstanceNotFound, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("ec2 instance mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetHackInstance(t *testing.T) {
	tests := map[string]struct {
		reader  *mockReader
		want    data.HackInstance
		wantErr bool
	}{
		"success with storage": {
			reader: &mockReader{
				hw: &v1alpha1.Hardware{
					Spec: v1alpha1.HardwareSpec{
						Metadata: &v1alpha1.HardwareMetadata{
							Instance: &v1alpha1.MetadataInstance{
								Storage: &v1alpha1.MetadataInstanceStorage{
									Disks: []*v1alpha1.MetadataInstanceStorageDisk{
										{
											Device:    "/dev/sda",
											WipeTable: true,
											Partitions: []*v1alpha1.MetadataInstanceStorageDiskPartition{
												{Label: "root", Number: 1, Size: 1000000},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"error from reader": {
			reader:  &mockReader{err: fmt.Errorf("fail")},
			wantErr: true,
		},
		"empty hardware": {
			reader: &mockReader{hw: &v1alpha1.Hardware{}},
			want:   data.HackInstance{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
b := New(tt.reader)
_, err := b.GetHackInstance(context.Background(), "10.0.0.1")

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestToEC2Instance(t *testing.T) {
	userData := "cloud-init data"
	tests := map[string]struct {
		hw   v1alpha1.Hardware
		want data.Ec2Instance
	}{
		"nil metadata": {
			hw:   v1alpha1.Hardware{},
			want: data.Ec2Instance{},
		},
		"nil instance": {
			hw: v1alpha1.Hardware{
				Spec: v1alpha1.HardwareSpec{
					Metadata: &v1alpha1.HardwareMetadata{},
				},
			},
			want: data.Ec2Instance{},
		},
		"facility only": {
			hw: v1alpha1.Hardware{
				Spec: v1alpha1.HardwareSpec{
					Metadata: &v1alpha1.HardwareMetadata{
						Facility: &v1alpha1.MetadataFacility{
							PlanSlug:     "plan-a",
							FacilityCode: "dc1",
						},
					},
				},
			},
			want: data.Ec2Instance{
				Metadata: data.Metadata{
					Plan:     "plan-a",
					Facility: "dc1",
				},
			},
		},
		"userdata": {
			hw: v1alpha1.Hardware{
				Spec: v1alpha1.HardwareSpec{
					UserData: &userData,
				},
			},
			want: data.Ec2Instance{
				Userdata: "cloud-init data",
			},
		},
		"first matching IPs chosen": {
			hw: v1alpha1.Hardware{
				Spec: v1alpha1.HardwareSpec{
					Metadata: &v1alpha1.HardwareMetadata{
						Instance: &v1alpha1.MetadataInstance{
							Ips: []*v1alpha1.MetadataInstanceIP{
								{Address: "pub4-first", Family: 4, Public: true},
								{Address: "pub4-second", Family: 4, Public: true},
								{Address: "priv4-first", Family: 4, Public: false},
								{Address: "priv4-second", Family: 4, Public: false},
								{Address: "pub6-first", Family: 6},
								{Address: "pub6-second", Family: 6},
							},
						},
					},
				},
			},
			want: data.Ec2Instance{
				Metadata: data.Metadata{
					PublicIPv4: "pub4-first",
					LocalIPv4:  "priv4-first",
					PublicIPv6: "pub6-first",
				},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
got := toEC2Instance(tt.hw)
if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"nil error":     {err: nil, want: false},
		"generic error": {err: fmt.Errorf("oops"), want: false},
		"not found error": {
			err:  notFoundError{msg: "gone"},
			want: true,
		},
		"wrapped not found": {
			err:  fmt.Errorf("wrap: %w", notFoundError{msg: "gone"}),
			want: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
if got := isNotFound(tt.err); got != tt.want {
				t.Fatalf("isNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
