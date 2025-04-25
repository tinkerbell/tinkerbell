package machine

import (
	"fmt"
	"sort"
	"strings"

	tinkv1 "github.com/tinkerbell/tink/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrastructurev1 "github.com/tinkerbell/cluster-api-provider-tinkerbell/api/v1beta1"
)

const (
	// HardwareOwnerNameLabel is a label set by either CAPT controllers or Tinkerbell controller to indicate
	// that given hardware takes part of at least one workflow.
	HardwareOwnerNameLabel = "v1alpha1.tinkerbell.org/ownerName"

	// HardwareOwnerNamespaceLabel is a label set by either CAPT controllers or Tinkerbell controller to indicate
	// that given hardware takes part of at least one workflow.
	HardwareOwnerNamespaceLabel = "v1alpha1.tinkerbell.org/ownerNamespace"

	// HardwareProvisionedAnnotation signifies that the Hardware with this annotation has be provisioned by CAPT.
	HardwareProvisionedAnnotation = "v1alpha1.tinkerbell.org/provisioned"
)

var (
	// ErrNoHardwareAvailable is the error returned when there is no hardware available for provisioning.
	ErrNoHardwareAvailable = fmt.Errorf("no hardware available")
	// ErrHardwareIsNil is the error returned when the given hardware resource is nil.
	ErrHardwareIsNil = fmt.Errorf("given Hardware object is nil")
	// ErrHardwareMissingInterfaces is the error returned when the referenced hardware does not have any
	// network interfaces defined.
	ErrHardwareMissingInterfaces = fmt.Errorf("hardware has no interfaces defined")
	// ErrHardwareFirstInterfaceNotDHCP is the error returned when the referenced hardware does not have it's
	// first network interface configured for DHCP.
	ErrHardwareFirstInterfaceNotDHCP = fmt.Errorf("hardware's first interface has no DHCP address defined")
	// ErrHardwareFirstInterfaceDHCPMissingIP is the error returned when the referenced hardware does not have a
	// DHCP IP address assigned for it's first interface.
	ErrHardwareFirstInterfaceDHCPMissingIP = fmt.Errorf("hardware's first interface has no DHCP IP address defined")
	// ErrHardwareMissingDiskConfiguration is returned when the referenced hardware is missing
	// disk configuration.
	ErrHardwareMissingDiskConfiguration = fmt.Errorf("disk configuration is required")
)

// hardwareIP returns the IP address of the first network interface of the given hardware.
func hardwareIP(hardware *tinkv1.Hardware) (string, error) {
	if hardware == nil {
		return "", ErrHardwareIsNil
	}

	if len(hardware.Spec.Interfaces) == 0 {
		return "", ErrHardwareMissingInterfaces
	}

	if hardware.Spec.Interfaces[0].DHCP == nil {
		return "", ErrHardwareFirstInterfaceNotDHCP
	}

	if hardware.Spec.Interfaces[0].DHCP.IP == nil {
		return "", ErrHardwareFirstInterfaceDHCPMissingIP
	}

	if hardware.Spec.Interfaces[0].DHCP.IP.Address == "" {
		return "", ErrHardwareFirstInterfaceDHCPMissingIP
	}

	return hardware.Spec.Interfaces[0].DHCP.IP.Address, nil
}

// patchHardwareStates patches a hardware's metadata and instance states.
func (scope *machineReconcileScope) patchHardwareAnnotations(hw *tinkv1.Hardware, annotations map[string]string) error {
	patchHelper, err := patch.NewHelper(hw, scope.client)
	if err != nil {
		return fmt.Errorf("initializing patch helper for selected hardware: %w", err)
	}

	if hw.ObjectMeta.Annotations == nil {
		hw.ObjectMeta.Annotations = map[string]string{}
	}

	for k, v := range annotations {
		hw.ObjectMeta.Annotations[k] = v
	}

	if err := patchHelper.Patch(scope.ctx, hw); err != nil {
		return fmt.Errorf("patching Hardware object: %w", err)
	}

	return nil
}

func (scope *machineReconcileScope) takeHardwareOwnership(hw *tinkv1.Hardware) error {
	if len(hw.ObjectMeta.Labels) == 0 {
		hw.ObjectMeta.Labels = map[string]string{}
	}

	hw.ObjectMeta.Labels[HardwareOwnerNameLabel] = scope.tinkerbellMachine.Name
	hw.ObjectMeta.Labels[HardwareOwnerNamespaceLabel] = scope.tinkerbellMachine.Namespace

	// Add finalizer to hardware as well to make sure we release it before Machine object is removed.
	controllerutil.AddFinalizer(hw, infrastructurev1.MachineFinalizer)

	if err := scope.client.Update(scope.ctx, hw); err != nil {
		return fmt.Errorf("updating Hardware object: %w", err)
	}

	return nil
}

func (scope *machineReconcileScope) ensureHardwareUserData(hw *tinkv1.Hardware, providerID string) error {
	userData := strings.ReplaceAll(scope.bootstrapCloudConfig, providerIDPlaceholder, providerID)

	if hw.Spec.UserData == nil || *hw.Spec.UserData != userData {
		patchHelper, err := patch.NewHelper(hw, scope.client)
		if err != nil {
			return fmt.Errorf("initializing patch helper for selected hardware: %w", err)
		}

		hw.Spec.UserData = &userData
		if err := patchHelper.Patch(scope.ctx, hw); err != nil {
			return fmt.Errorf("patching Hardware object: %w", err)
		}
	}

	return nil
}

func (scope *machineReconcileScope) ensureHardware() (*tinkv1.Hardware, error) {
	hw, err := scope.hardwareForMachine()
	if err != nil {
		return nil, fmt.Errorf("getting hardware: %w", err)
	}

	if err := scope.takeHardwareOwnership(hw); err != nil {
		return nil, fmt.Errorf("taking Hardware ownership: %w", err)
	}

	if scope.tinkerbellMachine.Spec.HardwareName == "" {
		scope.log.Info("Selected Hardware for machine", "Hardware name", hw.Name)
	}

	scope.tinkerbellMachine.Spec.HardwareName = hw.Name
	scope.tinkerbellMachine.Spec.ProviderID = fmt.Sprintf("tinkerbell://%s/%s", hw.Namespace, hw.Name)

	if err := scope.ensureHardwareUserData(hw, scope.tinkerbellMachine.Spec.ProviderID); err != nil {
		return nil, fmt.Errorf("ensuring Hardware user data: %w", err)
	}

	return hw, scope.setStatus(hw)
}

func (scope *machineReconcileScope) hardwareForMachine() (*tinkv1.Hardware, error) {
	// first query for hardware that's already assigned
	if hardware, err := scope.assignedHardware(); err != nil {
		return nil, err
	} else if hardware != nil {
		return hardware, nil
	}

	// then fallback to searching for new hardware
	hardwareSelector := scope.tinkerbellMachine.Spec.HardwareAffinity.DeepCopy()
	if hardwareSelector == nil {
		hardwareSelector = &infrastructurev1.HardwareAffinity{}
	}
	// if no terms are specified, we create an empty one to ensure we always query for non-selected hardware
	if len(hardwareSelector.Required) == 0 {
		hardwareSelector.Required = append(hardwareSelector.Required, infrastructurev1.HardwareAffinityTerm{})
	}

	var matchingHardware []tinkv1.Hardware

	// OR all of the required terms by selecting each individually, we could end up with duplicates in matchingHardware
	// but it doesn't matter
	for i := range hardwareSelector.Required {
		var matched tinkv1.HardwareList

		// add a selector for unselected hardware
		hardwareSelector.Required[i].LabelSelector.MatchExpressions = append(
			hardwareSelector.Required[i].LabelSelector.MatchExpressions,
			metav1.LabelSelectorRequirement{
				Key:      HardwareOwnerNameLabel,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			})

		selector, err := metav1.LabelSelectorAsSelector(&hardwareSelector.Required[i].LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("converting label selector: %w", err)
		}

		if err := scope.client.List(scope.ctx, &matched, &client.ListOptions{LabelSelector: selector}); err != nil {
			return nil, fmt.Errorf("listing hardware without owner: %w", err)
		}

		matchingHardware = append(matchingHardware, matched.Items...)
	}

	// finally sort by our preferred affinity terms
	cmp, err := byHardwareAffinity(matchingHardware, hardwareSelector.Preferred)
	if err != nil {
		return nil, fmt.Errorf("sorting hardware by preference: %w", err)
	}

	sort.Slice(matchingHardware, cmp)

	if len(matchingHardware) > 0 {
		return &matchingHardware[0], nil
	}
	// nothing was found
	return nil, ErrNoHardwareAvailable
}

// assignedHardware returns hardware that is already assigned. In the event of no hardware being assigned, it returns
// nil, nil.
func (scope *machineReconcileScope) assignedHardware() (*tinkv1.Hardware, error) {
	var selectedHardware tinkv1.HardwareList
	if err := scope.client.List(scope.ctx, &selectedHardware, client.MatchingLabels{
		HardwareOwnerNameLabel:      scope.tinkerbellMachine.Name,
		HardwareOwnerNamespaceLabel: scope.tinkerbellMachine.Namespace,
	}); err != nil {
		return nil, fmt.Errorf("listing hardware with owner: %w", err)
	}

	if len(selectedHardware.Items) > 0 {
		return &selectedHardware.Items[0], nil
	}

	return nil, nil
}

//nolint:lll
func byHardwareAffinity(hardware []tinkv1.Hardware, preferred []infrastructurev1.WeightedHardwareAffinityTerm) (func(i int, j int) bool, error) {
	scores := map[client.ObjectKey]int32{}
	// compute scores for each item based on the preferred term weights
	for _, term := range preferred {
		selector, err := metav1.LabelSelectorAsSelector(&term.HardwareAffinityTerm.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("constructing label selector: %w", err)
		}

		for i := range hardware {
			hw := &hardware[i]
			if selector.Matches(labels.Set(hw.Labels)) {
				scores[client.ObjectKeyFromObject(hw)] = term.Weight
			}
		}
	}

	return func(i, j int) bool {
		lhsScore := scores[client.ObjectKeyFromObject(&hardware[i])]
		rhsScore := scores[client.ObjectKeyFromObject(&hardware[j])]
		// sort by score in descending order
		if lhsScore > rhsScore {
			return true
		} else if lhsScore < rhsScore {
			return false
		}

		// just give a consistent ordering so we predictably pick one if scores are equal
		if hardware[i].Namespace != hardware[j].Namespace {
			return hardware[i].Namespace < hardware[j].Namespace
		}

		return hardware[i].Name < hardware[j].Name
	}, nil
}

func (scope *machineReconcileScope) releaseHardware(hw *tinkv1.Hardware) error {
	patchHelper, err := patch.NewHelper(hw, scope.client)
	if err != nil {
		return fmt.Errorf("initializing patch helper for selected hardware: %w", err)
	}

	delete(hw.ObjectMeta.Labels, HardwareOwnerNameLabel)
	delete(hw.ObjectMeta.Labels, HardwareOwnerNamespaceLabel)
	delete(hw.ObjectMeta.Annotations, HardwareProvisionedAnnotation)

	controllerutil.RemoveFinalizer(hw, infrastructurev1.MachineFinalizer)

	if err := patchHelper.Patch(scope.ctx, hw); err != nil {
		return fmt.Errorf("patching Hardware object: %w", err)
	}

	return nil
}

func (scope *machineReconcileScope) getHardwareForMachine(hardware *tinkv1.Hardware) error {
	namespacedName := types.NamespacedName{
		Name:      scope.tinkerbellMachine.Spec.HardwareName,
		Namespace: scope.tinkerbellMachine.Namespace,
	}

	if err := scope.client.Get(scope.ctx, namespacedName, hardware); err != nil {
		return fmt.Errorf("getting hardware: %w", err)
	}

	return nil
}
