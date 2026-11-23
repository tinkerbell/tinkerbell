package spec

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// The file and NATS transports decode Actions straight from YAML with gopkg.in/yaml.v3, which
// panics on struct tag flags it does not recognise rather than ignoring them. omitzero is one of
// those: it is an encoding/json feature, valid in the json tags here but not the yaml ones. This
// guards against it creeping back into a yaml tag and taking both transports down at startup.
func TestActionYAMLTagsAreDecodable(t *testing.T) {
	in := []byte(`
- id: "1"
  name: script action
  run: |
    echo hi
  shell: ["sh", "-u"]
  background: true
  timeoutSeconds: 30
  env:
    - key: KEY
      value: value
  volumes:
    - /dev:/dev
  namespaces:
    pid: host
    network: host
`)

	var actions []Action
	if err := yaml.Unmarshal(in, &actions); err != nil {
		t.Fatalf("yaml.Unmarshal() = %v, want nil", err)
	}
	if len(actions) != 1 {
		t.Fatalf("decoded %d actions, want 1", len(actions))
	}

	got := actions[0]
	if got.ID != "1" || got.Name != "script action" {
		t.Errorf("ID/Name = %q/%q, want \"1\"/\"script action\"", got.ID, got.Name)
	}
	if got.Run != "echo hi\n" {
		t.Errorf("Run = %q, want %q", got.Run, "echo hi\n")
	}
	if len(got.Shell) != 2 || got.Shell[0] != "sh" || got.Shell[1] != "-u" {
		t.Errorf("Shell = %v, want [sh -u]", got.Shell)
	}
	if !got.Background {
		t.Error("Background = false, want true")
	}
	if got.TimeoutSeconds != 30 {
		t.Errorf("TimeoutSeconds = %d, want 30", got.TimeoutSeconds)
	}
	if len(got.Env) != 1 || got.Env[0].Key != "KEY" || got.Env[0].Value != "value" {
		t.Errorf("Env = %v, want [{KEY value}]", got.Env)
	}
	if len(got.Volumes) != 1 || got.Volumes[0] != "/dev:/dev" {
		t.Errorf("Volumes = %v, want [/dev:/dev]", got.Volumes)
	}
	if got.Namespaces.PID != "host" || got.Namespaces.Network != "host" {
		t.Errorf("Namespaces = %+v, want both host", got.Namespaces)
	}
}

// Marshalling exercises the same tag parsing on the encode side, which the NATS transport relies on.
func TestActionYAMLMarshalOmitsEmptyFields(t *testing.T) {
	out, err := yaml.Marshal(Action{ID: "1", Name: "script", Run: "true\n"})
	if err != nil {
		t.Fatalf("yaml.Marshal() = %v, want nil", err)
	}

	var round Action
	if err := yaml.Unmarshal(out, &round); err != nil {
		t.Fatalf("yaml.Unmarshal() = %v, want nil", err)
	}
	if round.Run != "true\n" {
		t.Errorf("Run did not survive the round trip: %q", round.Run)
	}
}
