// api/v1beta1/libvirtmachine_types.go
package v1beta1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// LibvirtMachineSpec defines the desired state of LibvirtMachine
type LibvirtMachineSpec struct {
    // ProviderID will be the libvirt instance's ID in ProviderID format (e.g., libvirt://<UUID>).
    // +optional
    ProviderID *string `json:"providerID,omitempty"`

    // Image is the path or name of the VM base image to use.
    Image string `json:"image"`

    // NumCPU and MemoryMB define the VM hardware specs.
    NumCPU   int32 `json:"numCPU,omitempty"`
    MemoryMB int32 `json:"memoryMB,omitempty"`

}



type LibvirtCloudInitSpec struct {
    // UserData base64编码的用户数据
    UserData string `json:"userData,omitempty"`
    
    // NetworkData base64编码的网络数据
    NetworkData string `json:"networkData,omitempty"`
}

// LibvirtMachineStatus defines the observed state of LibvirtMachine
type LibvirtMachineStatus struct {
    // Ready indicates the VM has been created and is operational.
    Ready bool `json:"ready,omitempty"`

    // Addresses contains the associated addresses for the machine (e.g., IPs).
    // +optional
    Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`
}
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name"
//+kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name"
//+kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.instanceState"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready"

// LibvirtMachine is the Schema for the libvirtmachines API  
type LibvirtMachine struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   LibvirtMachineSpec   `json:"spec,omitempty"`
    Status LibvirtMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LibvirtMachineList contains a list of LibvirtMachine
type LibvirtMachineList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []LibvirtMachine `json:"items"`
}

