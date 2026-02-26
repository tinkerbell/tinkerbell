package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	validTemplate = `
version: "0.1"
name: hello_world_workflow
global_timeout: 600
tasks:
  - name: "hello world"
    worker: "{{.device_1}}"
    actions:
    - name: "hello_world"
      image: hello-world
      timeout: 60
`

	invalidTemplate = `
version: "0.1"
name: hello_world_workflow
global_timeout: 600
tasks:
  - name: "hello world"
    worker: "{{.device_1}}"
    actions:
  - name: "hello_world"
      image: hello-world
      timeout: 60
`

	veryLongName = "this is a very long string, that is used to test if the name is too long hahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhuhahahehehohohuhu"
)

func TestParse(t *testing.T) {
	testcases := []struct {
		name          string
		content       []byte
		expectedError bool
	}{
		{
			name:    "valid template",
			content: []byte(validTemplate),
		},
		{
			name:          "invalid template",
			content:       []byte(invalidTemplate),
			expectedError: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			res, err := parse(test.content)
			if err != nil {
				assert.Error(t, err)
				assert.Empty(t, res)
				return
			}
			assert.NoError(t, err)
			assert.NotEmpty(t, res)
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	testCases := []struct {
		name          string
		wf            *Workflow
		expectedError bool
	}{
		{
			name:          "template name is invalid",
			wf:            toWorkflow(withTemplateInvalidName()),
			expectedError: true,
		},
		{
			name:          "template name too long",
			wf:            toWorkflow(withTemplateLongName()),
			expectedError: true,
		},
		{
			name:          "template tasks is nil",
			wf:            toWorkflow(withTemplateNilTasks()),
			expectedError: true,
		},
		{
			name:          "template tasks is empty",
			wf:            toWorkflow(withTemplateEmptyTasks()),
			expectedError: true,
		},
		{
			name:          "task name is invalid",
			wf:            toWorkflow(withTaskInvalidName()),
			expectedError: true,
		},
		{
			name:          "task name is too long",
			wf:            toWorkflow(withTaskLongName()),
			expectedError: true,
		},
		{
			name:          "task name is duplicated",
			wf:            toWorkflow(withTaskDuplicateName()),
			expectedError: true,
		},
		{
			name:          "action name is invalid",
			wf:            toWorkflow(withActionInvalidName()),
			expectedError: true,
		},
		{
			name:          "action name is duplicated",
			wf:            toWorkflow(withActionDuplicateName()),
			expectedError: true,
		},
		{
			name:          "action name is too long",
			wf:            toWorkflow(withActionLongName()),
			expectedError: true,
		},
		{
			name:          "action image is invalid",
			wf:            toWorkflow(withActionInvalidImage()),
			expectedError: true,
		},
		{
			name: "valid task name",
			wf:   toWorkflow(),
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			err := validate(test.wf)
			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

type workflowModifier func(*Workflow)

func toWorkflow(m ...workflowModifier) *Workflow {
	wf := &Workflow{
		ID:            "ce2e62ed-826f-4485-a39f-a82bb74338e2",
		GlobalTimeout: 900,
		Name:          "ubuntu-provisioning",
		Version:       "0.1",
		Tasks: []Task{
			{
				Name:       "pre-installation",
				WorkerAddr: "08:00:27:00:00:01",
				Environment: map[string]string{
					"MIRROR_HOST": "192.168.1.2",
				},
				Volumes: []string{
					"/dev:/dev",
					"/dev/console:/dev/console",
					"/lib/firmware:/lib/firmware:ro",
				},
				Actions: []Action{
					{
						Name:    "disk-wipe",
						Image:   "disk-wipe",
						Timeout: 90,
					},
					{
						Name:    "disk-partition",
						Image:   "disk-partition",
						Timeout: 300,
						Volumes: []string{
							"/statedir:/statedir",
						},
					},
					{
						Name:    "install-root-fs",
						Image:   "install-root-fs",
						Timeout: 600,
					},
					{
						Name:    "install-grub",
						Image:   "install-grub",
						Timeout: 600,
						Volumes: []string{
							"/statedir:/statedir",
						},
					},
				},
			},
		},
	}
	for _, f := range m {
		f(wf)
	}
	return wf
}

// invalid task modifiers

func withTaskInvalidName() workflowModifier {
	return func(wf *Workflow) { wf.Tasks[0].Name = "" }
}

func withTaskLongName() workflowModifier {
	return func(wf *Workflow) {
		wf.Tasks[0].Name = veryLongName
	}
}

func withTaskDuplicateName() workflowModifier {
	return func(wf *Workflow) { wf.Tasks = append(wf.Tasks, wf.Tasks[0]) }
}

// invalid action modifiers

func withActionInvalidName() workflowModifier {
	return func(wf *Workflow) { wf.Tasks[0].Actions[0].Name = "" }
}

func withActionLongName() workflowModifier {
	return func(wf *Workflow) {
		wf.Tasks[0].Actions[0].Name = veryLongName
	}
}

func withActionDuplicateName() workflowModifier {
	return func(wf *Workflow) { wf.Tasks[0].Actions = append(wf.Tasks[0].Actions, wf.Tasks[0].Actions[0]) }
}

func withActionInvalidImage() workflowModifier {
	return func(wf *Workflow) { wf.Tasks[0].Actions[0].Image = "action-image-with-$#@-" }
}

// invalid template modifiers

func withTemplateInvalidName() workflowModifier {
	return func(wf *Workflow) { wf.Name = "" }
}

func withTemplateLongName() workflowModifier {
	return func(wf *Workflow) {
		wf.Name = veryLongName
	}
}

func withTemplateNilTasks() workflowModifier {
	return func(wf *Workflow) {
		wf.Tasks = nil
	}
}

func withTemplateEmptyTasks() workflowModifier {
	return func(wf *Workflow) {
		wf.Tasks = []Task{}
	}
}

func TestRenderTemplateHardwareWithToYaml(t *testing.T) {
	templateWithToYaml := `
version: "0.1"
name: yaml_func_workflow
global_timeout: 600
tasks:
  - name: "provision"
    worker: "{{.device_1}}"
    actions:
    - name: "apply-config"
      image: apply-config
      timeout: 60
      environment:
        CONFIG: |
          {{ .references.config | toYaml | nindent 10 }}
`
	hardware := map[string]interface{}{
		"device_1": "08:00:27:00:00:01",
		"references": map[string]interface{}{
			"config": map[string]interface{}{
				"hostname": "worker-1",
				"network": map[string]interface{}{
					"ip":      "192.168.1.10",
					"gateway": "192.168.1.1",
				},
			},
		},
	}

	wf, err := renderTemplateHardware("test-toYaml", templateWithToYaml, hardware)
	assert.NoError(t, err)
	assert.NotNil(t, wf)
	assert.Equal(t, "yaml_func_workflow", wf.Name)

	// Verify the rendered action environment contains YAML output
	env := wf.Tasks[0].Actions[0].Environment
	configVal, ok := env["CONFIG"]
	assert.True(t, ok, "CONFIG environment variable should be set")
	assert.Contains(t, configVal, "hostname: worker-1")
	assert.Contains(t, configVal, "gateway: 192.168.1.1")
	assert.Contains(t, configVal, "ip: 192.168.1.10")
}

func TestRenderTemplateHardwareWithFromYaml(t *testing.T) {
	templateWithFromYaml := `
version: "0.1"
name: yaml_func_workflow
global_timeout: 600
tasks:
  - name: "provision"
    worker: "{{.device_1}}"
    actions:
    - name: "apply-config"
      image: apply-config
      timeout: 60
      environment:
        HOSTNAME: "{{ (fromYaml .references.yamlConfig).hostname }}"
`
	hardware := map[string]interface{}{
		"device_1": "08:00:27:00:00:01",
		"references": map[string]interface{}{
			"yamlConfig": "hostname: worker-1\nip: 192.168.1.10",
		},
	}

	wf, err := renderTemplateHardware("test-fromYaml", templateWithFromYaml, hardware)
	assert.NoError(t, err)
	assert.NotNil(t, wf)

	env := wf.Tasks[0].Actions[0].Environment
	assert.Equal(t, "worker-1", env["HOSTNAME"])
}
