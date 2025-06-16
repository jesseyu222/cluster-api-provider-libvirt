// api/v1beta1/libvirtcluster_types.go
package v1beta1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// LibvirtClusterSpec defines the desired state of LibvirtCluster
type LibvirtClusterSpec struct {
    // URI是libvirt连接URI
     ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}



// LibvirtClusterStatus defines the observed state of LibvirtCluster
type LibvirtClusterStatus struct {
    Ready bool `json:"ready,omitempty"`
    FailureMessage *string `json:"failureMessage,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready"

// LibvirtCluster is the Schema for the libvirtclusters API
type LibvirtCluster struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   LibvirtClusterSpec   `json:"spec,omitempty"`
    Status LibvirtClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LibvirtClusterList contains a list of LibvirtCluster
type LibvirtClusterList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []LibvirtCluster `json:"items"`
}


