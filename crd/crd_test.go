package crd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	v1alpha2 "github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/validation"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	structuraldefaulting "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	apiservervalidation "k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utiljson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/validation/field"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	ktesting "k8s.io/client-go/testing"
	k8syaml "sigs.k8s.io/yaml"
)

func TestMigrateAndReady(t *testing.T) {
	var curCRDs sync.Map
	client := fake.NewSimpleClientset()
	// the Reactors are needed because the fake clientset does not support conditions on CRDs.
	// also important to note that reactors don't modify the actual CRD object in the clientset.
	// This is why need curCRDs. ktesting.CreateAction has access to the actual CRD object so we
	// use a create reactor to save the CRD object. The ktesting.GetAction doesn't have access to
	// the actual CRD object so we use curCRDs to get the CRD object.
	client.PrependReactor("create", "customresourcedefinitions", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		// get the CRD object from the action
		a, ok := action.(ktesting.CreateAction)
		if !ok {
			return false, nil, fmt.Errorf("expecting a CreateAction got type: %T", action)
		}
		// add the status conditions to the CRD object
		o, ok := a.GetObject().(*v1.CustomResourceDefinition)
		if !ok {
			return false, nil, fmt.Errorf("unexpected object type: %T", a.GetObject())
		}
		o.Status.Conditions = []v1.CustomResourceDefinitionCondition{
			{
				Type:   v1.Established,
				Status: v1.ConditionTrue,
				LastTransitionTime: metav1.Time{
					Time: time.Now(),
				},
			},
			{
				Type:   v1.NamesAccepted,
				Status: v1.ConditionTrue,
				LastTransitionTime: metav1.Time{
					Time: time.Now(),
				},
			},
		}

		curCRDs.Store(o.Name, o)
		return true, o, nil
	})
	client.PrependReactor("get", "customresourcedefinitions", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		a, ok := action.(ktesting.GetAction)
		if ok {
			if crd, ok := curCRDs.Load(a.GetName()); ok {
				return true, crd.(*v1.CustomResourceDefinition), nil
			}
		}
		return false, nil, nil
	})
	m, err := NewTinkerbell(func(t *Tinkerbell) { t.Client = client })
	if err != nil {
		t.Fatalf("failed to create Tinkerbell: %v", err)
	}
	if err := m.MigrateAndReady(context.Background()); err != nil {
		t.Errorf("failed to migrate CRDs: %v", err)
	}
}

// TestCRDsPassAPIServerValidation catches CRDs that generate cleanly but that
// the API server rejects, such as CEL rules that don't compile or exceed the
// estimated cost budget.
func TestCRDsPassAPIServerValidation(t *testing.T) {
	for version, crds := range CRDsByVersion {
		for name, raw := range crds {
			t.Run(version+"/"+name, func(t *testing.T) {
				crd := &v1.CustomResourceDefinition{}
				if err := k8syaml.UnmarshalStrict(raw, crd); err != nil {
					t.Fatalf("failed to decode CRD: %v", err)
				}
				v1.SetObjectDefaults_CustomResourceDefinition(crd)
				internal := &apiextensions.CustomResourceDefinition{}
				if err := v1.Convert_v1_CustomResourceDefinition_To_apiextensions_CustomResourceDefinition(crd, internal, nil); err != nil {
					t.Fatalf("failed to convert CRD: %v", err)
				}
				for _, err := range validation.ValidateCustomResourceDefinition(context.Background(), internal) {
					t.Error(err)
				}
			})
		}
	}
}

// newCRValidator returns a func that defaults a custom resource of the named CRD and validates
// it against its OpenAPI schema and CEL rules, as the API server does on create.
func newCRValidator(t *testing.T, version, name string) func(obj map[string]any) field.ErrorList {
	t.Helper()
	crd := &v1.CustomResourceDefinition{}
	if err := k8syaml.UnmarshalStrict(CRDsByVersion[version][name], crd); err != nil {
		t.Fatalf("failed to decode CRD: %v", err)
	}
	internal := &apiextensions.JSONSchemaProps{}
	if err := v1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(crd.Spec.Versions[0].Schema.OpenAPIV3Schema, internal, nil); err != nil {
		t.Fatalf("failed to convert schema: %v", err)
	}
	schemaValidator, _, err := apiservervalidation.NewSchemaValidator(internal)
	if err != nil {
		t.Fatalf("failed to build schema validator: %v", err)
	}
	s, err := structuralschema.NewStructural(internal)
	if err != nil {
		t.Fatalf("failed to build structural schema: %v", err)
	}
	celValidator := cel.NewValidator(s, true, celconfig.PerCallLimit)

	return func(obj map[string]any) field.ErrorList {
		structuraldefaulting.Default(obj, s)
		errs := apiservervalidation.ValidateCustomResource(nil, obj, schemaValidator)
		celErrs, _ := celValidator.Validate(context.Background(), field.NewPath(""), s, obj, nil, celconfig.RuntimeCELCostBudget)
		return append(errs, celErrs...)
	}
}

func TestBMCJobOperationsExactlyOne(t *testing.T) {
	validate := newCRValidator(t, "v1alpha2", "jobs.bmc.tinkerbell.org")

	tests := map[string]struct {
		operation map[string]any
		wantErr   bool
	}{
		"networkBootConfig only": {operation: map[string]any{"networkBootConfig": map[string]any{"pxeBootEnabled": true}}},
		"secureBoot only":        {operation: map[string]any{"secureBoot": map[string]any{"enable": true}}},
		"two operations":         {operation: map[string]any{"powerAction": "on", "secureBoot": map[string]any{"enable": true}}, wantErr: true},
		"no operation":           {operation: map[string]any{"timeoutSeconds": int64(10)}, wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			obj := map[string]any{
				"spec": map[string]any{
					"hardwareRef": map[string]any{"name": "hw"},
					"operations":  []any{tt.operation},
				},
			}
			if errs := validate(obj); (len(errs) > 0) != tt.wantErr {
				t.Errorf("got errors %v, want error: %v", errs, tt.wantErr)
			}
		})
	}
}

func TestHardwareNetworkInterfaceValidation(t *testing.T) {
	validate := newCRValidator(t, "v1alpha2", "hardware.tinkerbell.org")

	tests := map[string]struct {
		iface   map[string]any
		wantErr bool
	}{
		"dual stack": {iface: map[string]any{
			"ipam": map[string]any{
				"hostname": "node1.example.com",
				"vlanID":   int64(110),
				"ipv4":     map[string]any{"address": "192.0.2.10", "prefix": int64(24), "gateway": "192.0.2.1", "nameservers": []any{"192.0.2.53"}, "ntpServers": []any{"192.0.2.123"}},
				"ipv6":     map[string]any{"address": "2001:db8::10", "prefix": int64(64), "gateway": "fe80::1", "nameservers": []any{"2001:db8::53"}, "ntpServers": []any{"2001:db8::123"}},
			},
		}},
		"ipv4 address is IPv6":           {iface: map[string]any{"ipam": map[string]any{"ipv4": map[string]any{"address": "2001:db8::10", "prefix": int64(24)}}}, wantErr: true},
		"ipv6 gateway is IPv4":           {iface: map[string]any{"ipam": map[string]any{"ipv6": map[string]any{"gateway": "192.0.2.1"}}}, wantErr: true},
		"ipv4 nameserver is IPv6":        {iface: map[string]any{"ipam": map[string]any{"ipv4": map[string]any{"nameservers": []any{"2001:db8::53"}}}}, wantErr: true},
		"ipv6 ntp server is a name":      {iface: map[string]any{"ipam": map[string]any{"ipv6": map[string]any{"ntpServers": []any{"pool.ntp.org"}}}}, wantErr: true},
		"ipv4 prefix over 32":            {iface: map[string]any{"ipam": map[string]any{"ipv4": map[string]any{"address": "192.0.2.10", "prefix": int64(33)}}}, wantErr: true},
		"ipv4 address without prefix":    {iface: map[string]any{"ipam": map[string]any{"ipv4": map[string]any{"address": "192.0.2.10"}}}, wantErr: true},
		"hostname with backslash":        {iface: map[string]any{"ipam": map[string]any{"hostname": `node\1`}}, wantErr: true},
		"preferred over valid":           {iface: map[string]any{"dhcp": map[string]any{"ipv6": map[string]any{"preferredLifetimeSeconds": int64(7200), "validLifetimeSeconds": int64(3600)}}}, wantErr: true},
		"preferred over default valid":   {iface: map[string]any{"dhcp": map[string]any{"ipv6": map[string]any{"preferredLifetimeSeconds": int64(604801)}}}, wantErr: true},
		"refresh time under RFC minimum": {iface: map[string]any{"dhcp": map[string]any{"ipv6": map[string]any{"informationRefreshTimeSeconds": int64(300)}}}, wantErr: true},
		"bootFileURL tftp":               {iface: map[string]any{"netboot": map[string]any{"ipv6": map[string]any{"bootFileURL": "tftp://[2001:db8::1]/bootx64.efi"}}}},
		"bootFileURL unbracketed IPv6":   {iface: map[string]any{"netboot": map[string]any{"ipv6": map[string]any{"bootFileURL": "tftp://2001:db8::1/bootx64.efi"}}}, wantErr: true},
		"bootFileURL unsupported scheme": {iface: map[string]any{"netboot": map[string]any{"ipv6": map[string]any{"bootFileURL": "ftp://[2001:db8::1]/bootx64.efi"}}}, wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			obj := map[string]any{
				"spec": map[string]any{
					"networkInterfaces": map[string]any{"52:54:00:00:00:01": tt.iface},
				},
			}
			if errs := validate(obj); (len(errs) > 0) != tt.wantErr {
				t.Errorf("got errors %v, want error: %v", errs, tt.wantErr)
			}
		})
	}
}

func TestWorkflowTaskTargets(t *testing.T) {
	validate := newCRValidator(t, "v1alpha2", "workflows.tinkerbell.org")
	agent := func(id, key string) map[string]any {
		return map[string]any{"id": id, "authentication": map[string]any{"wireguard": map[string]any{"publicKey": key}}}
	}
	const key = "q4Hc8Q2rG0b1m9q2Wc7gB2sGq0bR3iX9Uo8nKp1vT0E="
	hardwareRef := map[string]any{"name": "machine1"}

	tests := map[string]struct {
		agents  []any
		task    map[string]any
		wantErr bool
	}{
		"hardware task":             {task: map[string]any{"hardwareRef": hardwareRef}},
		"workflow agent task":       {agents: []any{agent("switch-1", key)}, task: map[string]any{"agentID": "switch-1"}},
		"both targets":              {agents: []any{agent("switch-1", key)}, task: map[string]any{"agentID": "switch-1", "hardwareRef": hardwareRef}, wantErr: true},
		"no target":                 {task: map[string]any{}, wantErr: true},
		"agentID not in agents":     {agents: []any{agent("switch-1", key)}, task: map[string]any{"agentID": "switch-2"}, wantErr: true},
		"agentID without agents":    {task: map[string]any{"agentID": "switch-1"}, wantErr: true},
		"bootOptions on agent task": {agents: []any{agent("switch-1", key)}, task: map[string]any{"agentID": "switch-1", "bootOptions": map[string]any{"toggleNetboot": true}}, wantErr: true},
		"hardwareRef without name":  {task: map[string]any{"hardwareRef": map[string]any{}}, wantErr: true},
		"OnCheckIn on agent task":   {agents: []any{agent("switch-1", key)}, task: map[string]any{"agentID": "switch-1", "render": "OnCheckIn"}},
		"unknown render":            {task: map[string]any{"hardwareRef": hardwareRef, "render": "Later"}, wantErr: true},
		"invalid agent public key":  {agents: []any{agent("switch-1", "not-a-key")}, task: map[string]any{"agentID": "switch-1"}, wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt.task["taskRef"] = map[string]any{"name": "example"}
			spec := map[string]any{"tasks": []any{tt.task}}
			if tt.agents != nil {
				spec["agents"] = tt.agents
			}
			if errs := validate(map[string]any{"spec": spec}); (len(errs) > 0) != tt.wantErr {
				t.Errorf("got errors %v, want error: %v", errs, tt.wantErr)
			}
		})
	}

	t.Run("boot-only task built in Go", func(t *testing.T) {
		task := v1alpha2.WorkflowTask{
			HardwareRef: &v1alpha2.SimpleReference{Name: "machine1"},
			BootOptions: &v1alpha2.BootOptions{ToggleNetboot: true},
		}
		b, err := json.Marshal(task)
		if err != nil {
			t.Fatal(err)
		}
		obj := map[string]any{}
		if err := utiljson.Unmarshal(b, &obj); err != nil {
			t.Fatal(err)
		}
		if errs := validate(map[string]any{"spec": map[string]any{"tasks": []any{obj}}}); len(errs) > 0 {
			t.Errorf("unexpected errors: %v", errs)
		}
	})

	t.Run("render defaults to OnCreate", func(t *testing.T) {
		task := map[string]any{"hardwareRef": hardwareRef, "taskRef": map[string]any{"name": "example"}}
		if errs := validate(map[string]any{"spec": map[string]any{"tasks": []any{task}}}); len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if got := task["render"]; got != "OnCreate" {
			t.Errorf("render = %v, want OnCreate", got)
		}
	})
}
