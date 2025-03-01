package crd

import (
	_ "embed"
)

//go:embed bases/tinkerbell.org_hardware.yaml
var HardwareCRD []byte

//go:embed bases/tinkerbell.org_templates.yaml
var TemplateCRD []byte

//go:embed bases/tinkerbell.org_workflows.yaml
var WorkflowCRD []byte

//go:embed bases/bmc.tinkerbell.org_jobs.yaml
var JobCRD []byte

//go:embed bases/bmc.tinkerbell.org_machines.yaml
var MachineCRD []byte

//go:embed bases/bmc.tinkerbell.org_tasks.yaml
var TaskCRD []byte
