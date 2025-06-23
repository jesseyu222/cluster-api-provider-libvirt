// SPDX-License-Identifier: Apache-2.0
//
// NOTE: Run "make manifests" after editing this file so the CRDs are regenerated.
//
// This file defines the LibvirtMachine and LibvirtMachineTemplate APIs at the
// v1beta1 contract level, compatible with Cluster API v1beta1.

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

// -----------------------------------------------------------------------------
// LibvirtMachine Spec
// -----------------------------------------------------------------------------

// LibvirtMachineSpec defines the desired state of a Libvirt virtual machine.
// +k8s:openapi-gen=true
type LibvirtMachineSpec struct {
	ProviderID      *string `json:"providerID,omitempty"`
	Image           *string `json:"image,omitempty"`
	CPU             int32   `json:"cpu,omitempty"`
	MemoryMiB       int32   `json:"memoryMiB,omitempty"`
	Network         *string `json:"network,omitempty"`
	CloudInitSecret *string `json:"cloudInitSecret,omitempty"`
}

// -----------------------------------------------------------------------------
// LibvirtMachine Status
// -----------------------------------------------------------------------------

// LibvirtMachineStatus reflects the observed state of the VM.
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID"
// +kubebuilder:printcolumn:name="IPAddress",type="string",JSONPath=".status.addresses[0].address"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type LibvirtMachineStatus struct {
	Ready bool `json:"ready"`

	// Addresses are IPs assigned to the VM.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// FailureReason is set on terminal errors.
	// +optional
	FailureReason *capierrors.MachineStatusError `json:"failureReason,omitempty"`

	// FailureMessage is a human‑readable error.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Conditions standard CAPI conditions.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// GetConditions returns the list of conditions for a LibvirtMachine.
func (m *LibvirtMachine) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on a LibvirtMachine.
func (m *LibvirtMachine) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

// -----------------------------------------------------------------------------
// Kubernetes markers & top‑level objects
// -----------------------------------------------------------------------------

// +kubebuilder:resource:path=libvirtmachines,scope=Namespaced,shortName=lvmachine,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type LibvirtMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LibvirtMachineSpec   `json:"spec,omitempty"`
	Status LibvirtMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type LibvirtMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LibvirtMachine `json:"items"`
}

// -----------------------------------------------------------------------------
// LibvirtMachineTemplate (for ClusterClass/MachineDeployment)
// -----------------------------------------------------------------------------

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=libvirtmachinetemplates,scope=Namespaced,shortName=lvmachinetpl,categories=cluster-api
// +kubebuilder:subresource:status

type LibvirtMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LibvirtMachineTemplateSpec `json:"spec,omitempty"`
}

// LibvirtMachineTemplateSpec describes the template.
// +k8s:openapi-gen=true
type LibvirtMachineTemplateSpec struct {
	Template LibvirtMachineTemplateResource `json:"template"`
}

// LibvirtMachineTemplateResource holds metadata + spec for Machines.
// +k8s:openapi-gen=true
type LibvirtMachineTemplateResource struct {
	ObjectMeta metav1.ObjectMeta  `json:"metadata,omitempty"`
	Spec       LibvirtMachineSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type LibvirtMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LibvirtMachineTemplate `json:"items"`
}

func init() { //nolint:gochecknoinits
	SchemeBuilder.Register(
		&LibvirtMachine{}, &LibvirtMachineList{},
		&LibvirtMachineTemplate{}, &LibvirtMachineTemplateList{},
	)
}
