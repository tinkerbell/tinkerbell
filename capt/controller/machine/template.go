package machine

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"

	tinkv1 "github.com/tinkerbell/tink/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	// ErrMissingName is the error returned when the WorfklowTemplate Name is not specified.
	ErrMissingName = fmt.Errorf("name can't be empty")

	// ErrMissingImageURL is the error returned when the WorfklowTemplate ImageURL is not specified.
	ErrMissingImageURL = fmt.Errorf("imageURL can't be empty")
)

const (
	workflowTemplate = `
version: "0.1"
name: {{.Name}}
global_timeout: 6000
tasks:
  - name: "{{.Name}}"
    worker: "{{.DeviceTemplateName}}"
    volumes:
      - /dev:/dev
      - /dev/console:/dev/console
      - /lib/firmware:/lib/firmware:ro
    actions:
      - name: "stream image"
        image: quay.io/tinkerbell/actions/oci2disk
        timeout: 600
        environment:
          IMG_URL: {{.ImageURL}}
          DEST_DISK: {{.DestDisk}}
          COMPRESSED: true
      - name: "add tink cloud-init config"
        image: quay.io/tinkerbell/actions/writefile
        timeout: 90
        environment:
          DEST_DISK: {{.DestPartition}}
          FS_TYPE: ext4
          DEST_PATH: /etc/cloud/cloud.cfg.d/10_tinkerbell.cfg
          UID: 0
          GID: 0
          MODE: 0600
          DIRMODE: 0700
          CONTENTS: |
            datasource:
              Ec2:
                metadata_urls: ["{{.MetadataURL}}"]
                strict_id: false
            system_info:
              default_user:
                name: tink
                groups: [wheel, adm]
                sudo: ["ALL=(ALL) NOPASSWD:ALL"]
                shell: /bin/bash
            manage_etc_hosts: localhost
            warnings:
              dsid_missing_source: off
      - name: "add tink cloud-init ds-config"
        image: quay.io/tinkerbell/actions/writefile
        timeout: 90
        environment:
          DEST_DISK: {{.DestPartition}}
          FS_TYPE: ext4
          DEST_PATH: /etc/cloud/ds-identify.cfg
          UID: 0
          GID: 0
          MODE: 0600
          DIRMODE: 0700
          CONTENTS: |
            datasource: Ec2
      - name: "kexec image"
        image: ghcr.io/jacobweinstock/waitdaemon:0.2.1
        timeout: 90
        pid: host
        environment:
          BLOCK_DEVICE: {{.DestPartition}}
          FS_TYPE: ext4
          IMAGE: quay.io/tinkerbell/actions/kexec
          WAIT_SECONDS: 5
        volumes:
          - /var/run/docker.sock:/var/run/docker.sock
`
)

// WorkflowTemplate is a helper struct for rendering CAPT Template data.
type WorkflowTemplate struct {
	Name               string
	MetadataURL        string
	ImageURL           string
	DestDisk           string
	DestPartition      string
	DeviceTemplateName string
}

// Render renders workflow template for a given machine including user-data.
func (wt *WorkflowTemplate) Render() (string, error) {
	if wt.Name == "" {
		return "", ErrMissingName
	}

	if wt.ImageURL == "" {
		return "", ErrMissingImageURL
	}

	if wt.DeviceTemplateName == "" {
		wt.DeviceTemplateName = "{{.device_1}}"
	}

	tpl, err := template.New("template").Parse(workflowTemplate)
	if err != nil {
		return "", fmt.Errorf("unable to parse template: %w", err)
	}

	buf := &bytes.Buffer{}

	err = tpl.Execute(buf, wt)
	if err != nil {
		return "", fmt.Errorf("unable to execute template: %w", err)
	}

	return buf.String(), nil
}

func (scope *machineReconcileScope) templateExists() (bool, error) {
	namespacedName := types.NamespacedName{
		Name:      scope.tinkerbellMachine.Name,
		Namespace: scope.tinkerbellMachine.Namespace,
	}

	err := scope.client.Get(scope.ctx, namespacedName, &tinkv1.Template{})
	if err == nil {
		return true, nil
	}

	if !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("checking if template exists: %w", err)
	}

	return false, nil
}

func (scope *machineReconcileScope) createTemplate(hw *tinkv1.Hardware) error {
	if len(hw.Spec.Disks) < 1 {
		return ErrHardwareMissingDiskConfiguration
	}

	templateData := scope.tinkerbellMachine.Spec.TemplateOverride
	if templateData == "" {
		targetDisk := hw.Spec.Disks[0].Device
		targetDevice := firstPartitionFromDevice(targetDisk)

		imageURL, err := scope.imageURL()
		if err != nil {
			return fmt.Errorf("failed to generate imageURL: %w", err)
		}

		metadataIP := os.Getenv("TINKERBELL_IP")
		if metadataIP == "" {
			metadataIP = "192.168.1.1"
		}

		metadataURL := fmt.Sprintf("http://%s:50061", metadataIP)

		workflowTemplate := WorkflowTemplate{
			Name:          scope.tinkerbellMachine.Name,
			MetadataURL:   metadataURL,
			ImageURL:      imageURL,
			DestDisk:      targetDisk,
			DestPartition: targetDevice,
		}

		templateData, err = workflowTemplate.Render()
		if err != nil {
			return fmt.Errorf("rendering template: %w", err)
		}
	}

	templateObject := &tinkv1.Template{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scope.tinkerbellMachine.Name,
			Namespace: scope.tinkerbellMachine.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					Kind:       "TinkerbellMachine",
					Name:       scope.tinkerbellMachine.Name,
					UID:        scope.tinkerbellMachine.ObjectMeta.UID,
				},
			},
		},
		Spec: tinkv1.TemplateSpec{
			Data: &templateData,
		},
	}

	if err := scope.client.Create(scope.ctx, templateObject); err != nil {
		return fmt.Errorf("creating Tinkerbell template: %w", err)
	}

	return nil
}

func firstPartitionFromDevice(device string) string {
	nvmeDevice := regexp.MustCompile(`^/dev/nvme\d+n\d+$`)
	emmcDevice := regexp.MustCompile(`^/dev/mmcblk\d+$`)

	switch {
	case nvmeDevice.MatchString(device), emmcDevice.MatchString(device):
		return fmt.Sprintf("%sp1", device)
	default:
		return fmt.Sprintf("%s1", device)
	}
}

func (scope *machineReconcileScope) ensureTemplate(hardware *tinkv1.Hardware) error {
	// TODO: should this reconccile the template instead of just ensuring it exists?
	templateExists, err := scope.templateExists()
	if err != nil {
		return fmt.Errorf("checking if Template exists: %w", err)
	}

	if templateExists {
		return nil
	}

	scope.log.Info("template for machine does not exist, creating")

	return scope.createTemplate(hardware)
}

// removeTemplate makes sure template for TinkerbellMachine has been cleaned up.
func (scope *machineReconcileScope) removeTemplate() error {
	namespacedName := types.NamespacedName{
		Name:      scope.tinkerbellMachine.Name,
		Namespace: scope.tinkerbellMachine.Namespace,
	}

	template := &tinkv1.Template{}

	err := scope.client.Get(scope.ctx, namespacedName, template)
	if err != nil {
		if apierrors.IsNotFound(err) {
			scope.log.Info("Template already removed", "name", namespacedName)

			return nil
		}

		return fmt.Errorf("checking if template exists: %w", err)
	}

	scope.log.Info("Removing Template", "name", namespacedName)

	if err := scope.client.Delete(scope.ctx, template); err != nil {
		return fmt.Errorf("ensuring template has been removed: %w", err)
	}

	return nil
}

func (scope *machineReconcileScope) imageURL() (string, error) {
	imageLookupFormat := scope.tinkerbellMachine.Spec.ImageLookupFormat
	if imageLookupFormat == "" {
		imageLookupFormat = scope.tinkerbellCluster.Spec.ImageLookupFormat
	}

	imageLookupBaseRegistry := scope.tinkerbellMachine.Spec.ImageLookupBaseRegistry
	if imageLookupBaseRegistry == "" {
		imageLookupBaseRegistry = scope.tinkerbellCluster.Spec.ImageLookupBaseRegistry
	}

	imageLookupOSDistro := scope.tinkerbellMachine.Spec.ImageLookupOSDistro
	if imageLookupOSDistro == "" {
		imageLookupOSDistro = scope.tinkerbellCluster.Spec.ImageLookupOSDistro
	}

	imageLookupOSVersion := scope.tinkerbellMachine.Spec.ImageLookupOSVersion
	if imageLookupOSVersion == "" {
		imageLookupOSVersion = scope.tinkerbellCluster.Spec.ImageLookupOSVersion
	}

	return imageURL(
		imageLookupFormat,
		imageLookupBaseRegistry,
		imageLookupOSDistro,
		imageLookupOSVersion,
		*scope.machine.Spec.Version,
	)
}

func imageURL(imageFormat, baseRegistry, osDistro, osVersion, kubernetesVersion string) (string, error) {
	type image struct {
		BaseRegistry      string
		OSDistro          string
		OSVersion         string
		KubernetesVersion string
	}

	imageParams := image{
		BaseRegistry:      baseRegistry,
		OSDistro:          strings.ToLower(osDistro),
		OSVersion:         strings.ReplaceAll(osVersion, ".", ""),
		KubernetesVersion: kubernetesVersion,
	}

	var buf bytes.Buffer

	template, err := template.New("image").Parse(imageFormat)
	if err != nil {
		return "", fmt.Errorf("failed to create template from string %q: %w", imageFormat, err)
	}

	if err := template.Execute(&buf, imageParams); err != nil {
		return "", fmt.Errorf("failed to populate template %q: %w", imageFormat, err)
	}

	return buf.String(), nil
}
