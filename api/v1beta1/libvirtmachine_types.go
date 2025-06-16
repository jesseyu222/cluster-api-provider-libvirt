// api/v1beta1/libvirtmachine_types.go
package v1beta1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// -----------------------------------------------------------------------------
// Spec
// -----------------------------------------------------------------------------

// LibvirtMachineSpec describes the desired state of a libvirt‑backed virtual
// machine.
type LibvirtMachineSpec struct {
    // ProviderID is the domain UUID expressed in ProviderID format, e.g.
    //   libvirt://<uuid>
    // +optional
    ProviderID *string `json:"providerID,omitempty"`

    // Image identifies the base image that should be used. It can be a local
    // path, URL, or a short name resolvable by the reconciler.
    Image string `json:"image"`

    // NumCPU is the number of virtual CPUs assigned to the guest.
    // +kubebuilder:default:=2
    // +optional
    NumCPU int32 `json:"numCPU,omitempty"`

    // MemoryMB is the amount of RAM, in MiB, assigned to the guest.
    // +kubebuilder:default:=2048
    // +optional
    MemoryMB int32 `json:"memoryMB,omitempty"`

    // CloudInit contains user‑data and network‑data that will be injected via
    // cloud‑init.
    // +optional
    CloudInit *LibvirtCloudInitSpec `json:"cloudInit,omitempty"`
}

// LibvirtCloudInitSpec bundles base64‑encoded cloud‑init payloads.
type LibvirtCloudInitSpec struct {
    // UserData is the cloud‑init user‑data payload.
    // +optional
    UserData string `json:"userData,omitempty"`

    // NetworkData is the cloud‑init network‑data payload.
    // +optional
    NetworkData string `json:"networkData,omitempty"`
}

// -----------------------------------------------------------------------------
// Status
// -----------------------------------------------------------------------------

// LibvirtMachineStatus captures the observed state of the VM.
type LibvirtMachineStatus struct {
    // Ready is true when the domain has been created and is running.
    Ready bool `json:"ready,omitempty"`

    // Addresses lists IP addresses associated with the VM.
    // +optional
    Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`
    // InstanceState holds current libvirt domain state.
    // +optional
    InstanceState *string `json:"instanceState,omitempty"`
}

// -----------------------------------------------------------------------------
// +kubebuilder markers
// -----------------------------------------------------------------------------
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.instanceState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready"

// LibvirtMachine is the root API object representing a libvirt‑backed
// Cluster‑API Machine.
type LibvirtMachine struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   LibvirtMachineSpec   `json:"spec,omitempty"`
    Status LibvirtMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LibvirtMachineList contains a list of LibvirtMachine objects.
type LibvirtMachineList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []LibvirtMachine `json:"items"`
}
