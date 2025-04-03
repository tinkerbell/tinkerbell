package grpc

import (
	"testing"

	"github.com/tinkerbell/tinkerbell/pkg/proto"
)

func TestFlattenAttributes(t *testing.T) {
	tests := []struct {
		name     string
		input    *proto.AgentAttributes
		expected map[string]string
	}{
		{
			name:     "nil attributes",
			input:    nil,
			expected: map[string]string{},
		},
		{
			name: "CPU attributes",
			input: &proto.AgentAttributes{
				Cpu: &proto.CPU{
					TotalCores:   toPtr(uint32(8)),
					TotalThreads: toPtr(uint32(16)),
					Processors: []*proto.Processor{
						{
							Id:      toPtr(uint32(36)),
							Cores:   toPtr(uint32(4)),
							Threads: toPtr(uint32(8)),
							Vendor:  toPtr("Intel"),
							Model:   toPtr("Xeon"),
						},
					},
				},
			},
			expected: map[string]string{
				"tinkerbell.org/cpu-totalCores":   "8",
				"tinkerbell.org/cpu-totalThreads": "16",
				"tinkerbell.org/cpu-0-id":         "36",
				"tinkerbell.org/cpu-0-cores":      "4",
				"tinkerbell.org/cpu-0-threads":    "8",
				"tinkerbell.org/cpu-0-vendor":     "Intel",
				"tinkerbell.org/cpu-0-model":      "Xeon",
			},
		},
		{
			name: "Memory attributes",
			input: &proto.AgentAttributes{
				Memory: &proto.Memory{
					Total:  toPtr("32GB"),
					Usable: toPtr("31GB"),
				},
			},
			expected: map[string]string{
				"tinkerbell.org/memory-total":  "32GB",
				"tinkerbell.org/memory-usable": "31GB",
			},
		},
		{
			name: "Chassis attributes",
			input: &proto.AgentAttributes{
				Chassis: &proto.Chassis{
					Serial: toPtr("12345"),
					Vendor: toPtr("Dell"),
				},
			},
			expected: map[string]string{
				"tinkerbell.org/chassis-serial": "12345",
				"tinkerbell.org/chassis-vendor": "Dell",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := flattenAttributes(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d attributes, got %d", len(tt.expected), len(result))
			}
			for key, value := range tt.expected {
				if result[key] != value {
					t.Errorf("expected %s=%s, got %s=%s", key, value, key, result[key])
				}
			}
		})
	}
}

func TestAddNonNil(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    *string
		expected map[string]string
	}{
		{
			name:     "non-nil value",
			key:      "test-key",
			value:    toPtr("test-value"),
			expected: map[string]string{"test-key": "test-value"},
		},
		{
			name:     "nil value",
			key:      "test-key",
			value:    nil,
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]string)
			addNonNil(result, tt.key, tt.value)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d attributes, got %d", len(tt.expected), len(result))
			}
			for key, value := range tt.expected {
				if result[key] != value {
					t.Errorf("expected %s=%s, got %s=%s", key, value, key, result[key])
				}
			}
		})
	}
}
