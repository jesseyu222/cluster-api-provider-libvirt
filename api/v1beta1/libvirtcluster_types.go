package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// -----------------------------------------------------------------------------
// Spec
// -----------------------------------------------------------------------------

type LibvirtClusterSpec struct {
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
	// +optional
	URI *string `json:"uri,omitempty"`
	// +optional
	Network *string `json:"network,omitempty"`
}

// -----------------------------------------------------------------------------
// Status
// -----------------------------------------------------------------------------

type LibvirtClusterStatus struct {
	Ready bool `json:"ready,omitempty"`
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
}

// -----------------------------------------------------------------------------
// +kubebuilder markers
// -----------------------------------------------------------------------------

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready"

type LibvirtCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LibvirtClusterSpec   `json:"spec,omitempty"`
	Status LibvirtClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type LibvirtClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LibvirtCluster `json:"items"`
}
