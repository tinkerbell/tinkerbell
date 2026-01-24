// +kubebuilder:object:generate=true
// +groupName=bmc.tinkerbell.org
// +versionName:=v1alpha2
package api

import (
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha2 "github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell/bmc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

var (
	GroupVersion = schema.GroupVersion{Group: "bmc.tinkerbell.org", Version: "v1alpha2"}

	// schemeBuilder is used to add go types to the GroupVersionKind scheme.
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = schemeBuilder.AddToScheme

	objectTypes = []runtime.Object{}
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, objectTypes...)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=machines,scope=Namespaced,categories=tinkerbell,shortName=m,singular=machine
// +kubebuilder:conversion:webhook
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io=
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io/move=
// +kubebuilder:printcolumn:JSONPath=".status.conditions[?(@.type==\"Contactable\")].status",name=contactable,type=string,description="The contactable status of the machine",priority=1
// +kubebuilder:printcolumn:JSONPath=".status.conditions[?(@.type==\"PowerState\")].status",name=power-state,type=string,description="The power state of the machine",priority=1

type Machine v1alpha2.Machine

// ConvertTo converts a v1alpha2 Machine (Spoke) to the v1alpha1 version (Hub).
func (m *Machine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha1.Machine)

	// Copy ObjectMeta
	dst.ObjectMeta = m.ObjectMeta

	// Convert Spec fields
	dst.Spec.Connection = v1alpha1.Connection{
		Host: m.Spec.Connection.Host,
		AuthSecretRef: v1.SecretReference{
			Name:      m.Spec.Connection.AuthSecretRef.Name,
			Namespace: m.Spec.Connection.AuthSecretRef.Namespace,
		},
		InsecureTLS: m.Spec.Connection.InsecureTLS,
	}
	if m.Spec.Connection.ProviderOptions != nil {
		dst.Spec.Connection.ProviderOptions = v1alpha2Tov1alpha1ProviderOptions(m.Spec.Connection.ProviderOptions)
	}

	// Convert Status fields
	for _, cond := range m.Status.Conditions {
		if cond.Type == v1alpha2.ConditionTypeMachinePowerState {
			dst.Status.Power = v1alpha1.PowerState(cond.Status)
			break
		}
		switch cond.Type {
		case v1alpha2.ConditionTypeMachineContactable:
			dst.Status.Conditions = append(dst.Status.Conditions, v1alpha1.MachineCondition{
				Type:           v1alpha1.Contactable,
				Status:         v1alpha1.ConditionStatus(cond.Status),
				LastUpdateTime: cond.LastUpdateTime,
				Message:        cond.Message,
			})
		case v1alpha2.ConditionTypeMachinePowerState:
			dst.Status.Power = v1alpha1.PowerState(cond.Status)
		}
	}

	return nil
}

// ConvertFrom converts a v1alpha1 Machine (Hub) to the v1alpha2 version (Spoke).
func (m *Machine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha1.Machine)

	// Copy ObjectMeta
	m.ObjectMeta = src.ObjectMeta

	// Convert Spec fields
	m.Spec.Connection = v1alpha2.Connection{
		Host: src.Spec.Connection.Host,
		AuthSecretRef: v1alpha2.SecretReference{
			Name:      src.Spec.Connection.AuthSecretRef.Name,
			Namespace: src.Spec.Connection.AuthSecretRef.Namespace,
		},
		InsecureTLS:     src.Spec.Connection.InsecureTLS,
		ProviderOptions: v1alpha1Tov1alpha2ProviderOptions(src.Spec.Connection.ProviderOptions),
	}

	// Convert Status fields
	for _, cond := range src.Status.Conditions {
		if cond.Type == v1alpha1.Contactable {
			m.Status.Conditions = append(m.Status.Conditions, v1alpha2.Condition{
				Type:               v1alpha2.ConditionTypeMachineContactable,
				Status:             v1alpha2.ConditionStatus(cond.Status),
				LastUpdateTime:     cond.LastUpdateTime,
				Message:            cond.Message,
				ObservedGeneration: src.Generation,
			})
		}
	}
	if src.Status.Power != "" {
		m.Status.Conditions = append(m.Status.Conditions, v1alpha2.Condition{
			Type:               v1alpha2.ConditionTypeMachinePowerState,
			Status:             v1alpha2.ConditionStatus(src.Status.Power),
			LastUpdateTime:     metav1.Now(),
			ObservedGeneration: src.Generation,
		})
	}

	return nil
}

func v1alpha2Tov1alpha1ProviderOptions(src *v1alpha2.ProviderOptions) *v1alpha1.ProviderOptions {
	if src == nil {
		return nil
	}

	dst := &v1alpha1.ProviderOptions{}
	if src.PreferredOrder != nil {
		dst.PreferredOrder = make([]v1alpha1.ProviderName, len(src.PreferredOrder))
		for i, provider := range src.PreferredOrder {
			dst.PreferredOrder[i] = v1alpha1.ProviderName(provider)
		}
	}
	if src.IntelAMT != nil {
		dst.IntelAMT = &v1alpha1.IntelAMTOptions{
			Port:       src.IntelAMT.Port,
			HostScheme: src.IntelAMT.HostScheme,
		}
	}

	if src.IPMITOOL != nil {
		dst.IPMITOOL = &v1alpha1.IPMITOOLOptions{
			Port:        src.IPMITOOL.Port,
			CipherSuite: src.IPMITOOL.CipherSuite,
		}
	}
	if src.Redfish != nil {
		dst.Redfish = &v1alpha1.RedfishOptions{
			Port:         src.Redfish.Port,
			UseBasicAuth: src.Redfish.UseBasicAuth,
			SystemName:   src.Redfish.SystemName,
		}
	}
	if src.RPC != nil {
		dst.RPC = &v1alpha1.RPCOptions{
			ConsumerURL:              src.RPC.ConsumerURL,
			LogNotificationsDisabled: src.RPC.LogNotificationsDisabled,
		}
		if src.RPC.Request != nil {
			dst.RPC.Request = &v1alpha1.RequestOpts{
				HTTPContentType: src.RPC.Request.HTTPContentType,
				HTTPMethod:      src.RPC.Request.HTTPMethod,
				TimestampFormat: src.RPC.Request.TimestampFormat,
				TimestampHeader: src.RPC.Request.TimestampHeader,
				StaticHeaders:   src.RPC.Request.StaticHeaders,
			}
			if src.RPC.Signature != nil {
				dst.RPC.Signature = &v1alpha1.SignatureOpts{
					HeaderName:                 src.RPC.Signature.HeaderName,
					AppendAlgoToHeaderDisabled: src.RPC.Signature.AppendAlgoToHeaderDisabled,
					IncludedPayloadHeaders:     src.RPC.Signature.IncludedPayloadHeaders,
				}
			}
			if src.RPC.HMAC != nil {
				dst.RPC.HMAC = &v1alpha1.HMACOpts{
					PrefixSigDisabled: src.RPC.HMAC.PrefixSigDisabled,
				}
				for algo, secrets := range src.RPC.HMAC.Secrets {
					dst.RPC.HMAC.Secrets = make(v1alpha1.HMACSecrets)
					for _, v := range secrets {
						dst.RPC.HMAC.Secrets[v1alpha1.HMACAlgorithm(algo)] = append(dst.RPC.HMAC.Secrets[v1alpha1.HMACAlgorithm(algo)], v1.SecretReference{Name: v.Name, Namespace: v.Namespace})
					}
				}
			}
			if src.RPC.Experimental != nil {
				dst.RPC.Experimental = &v1alpha1.ExperimentalOpts{
					CustomRequestPayload: src.RPC.Experimental.CustomRequestPayload,
					DotPath:              src.RPC.Experimental.DotPath,
				}
			}
		}
	}

	return nil
}

func v1alpha1Tov1alpha2ProviderOptions(src *v1alpha1.ProviderOptions) *v1alpha2.ProviderOptions {
	if src == nil {
		return nil
	}

	dst := &v1alpha2.ProviderOptions{}
	if src.PreferredOrder != nil {
		dst.PreferredOrder = make([]v1alpha2.ProviderName, len(src.PreferredOrder))
		for i, provider := range src.PreferredOrder {
			dst.PreferredOrder[i] = v1alpha2.ProviderName(provider)
		}
	}
	if src.IntelAMT != nil {
		dst.IntelAMT = &v1alpha2.IntelAMTOptions{
			Port:       src.IntelAMT.Port,
			HostScheme: src.IntelAMT.HostScheme,
		}
	}

	if src.IPMITOOL != nil {
		dst.IPMITOOL = &v1alpha2.IPMITOOLOptions{
			Port:        src.IPMITOOL.Port,
			CipherSuite: src.IPMITOOL.CipherSuite,
		}
	}
	if src.Redfish != nil {
		dst.Redfish = &v1alpha2.RedfishOptions{
			Port:         src.Redfish.Port,
			UseBasicAuth: src.Redfish.UseBasicAuth,
			SystemName:   src.Redfish.SystemName,
		}
	}
	if src.RPC != nil {
		dst.RPC = &v1alpha2.RPCOptions{
			ConsumerURL:              src.RPC.ConsumerURL,
			LogNotificationsDisabled: src.RPC.LogNotificationsDisabled,
		}
		if src.RPC.Request != nil {
			dst.RPC.Request = &v1alpha2.RequestOpts{
				HTTPContentType: src.RPC.Request.HTTPContentType,
				HTTPMethod:      src.RPC.Request.HTTPMethod,
				StaticHeaders:   src.RPC.Request.StaticHeaders,
				TimestampFormat: src.RPC.Request.TimestampFormat,
				TimestampHeader: src.RPC.Request.TimestampHeader,
			}
		}
		if src.RPC.Signature != nil {
			dst.RPC.Signature = &v1alpha2.SignatureOpts{
				HeaderName:                 src.RPC.Signature.HeaderName,
				AppendAlgoToHeaderDisabled: src.RPC.Signature.AppendAlgoToHeaderDisabled,
				IncludedPayloadHeaders:     src.RPC.Signature.IncludedPayloadHeaders,
			}
		}
		if src.RPC.HMAC != nil {
			dst.RPC.HMAC = &v1alpha2.HMACOpts{
				PrefixSigDisabled: src.RPC.HMAC.PrefixSigDisabled,
			}
			for algo, secrets := range src.RPC.HMAC.Secrets {
				dst.RPC.HMAC.Secrets = make(v1alpha2.HMACSecrets)
				for _, v := range secrets {
					dst.RPC.HMAC.Secrets[v1alpha2.HMACAlgorithm(algo)] = append(dst.RPC.HMAC.Secrets[v1alpha2.HMACAlgorithm(algo)], v1alpha2.SecretReference{Name: v.Name, Namespace: v.Namespace})
				}
			}
		}
		if src.RPC.Experimental != nil {
			dst.RPC.Experimental = &v1alpha2.ExperimentalOpts{
				CustomRequestPayload: src.RPC.Experimental.CustomRequestPayload,
				DotPath:              src.RPC.Experimental.DotPath,
			}
		}

	}

	return nil
}
