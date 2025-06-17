// SPDX-License-Identifier: Apache-2.0
//
// NOTE: Boilerplate only.  Ignore this file except to declare the API schema.
//
// Package v1beta1 contains API Schema definitions for the Libvirt infrastructure
// provider. The API follows the Cluster API contract v1beta1.
//
// +kubebuilder:object:generate=true
// +groupName=infrastructure.cluster.x-k8s.io
package v1beta1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// -----------------------------------------------------------------------------
// Constants & helper types
// -----------------------------------------------------------------------------

const (
    // LibvirtClusterFinalizer allows LibvirtClusterReconciler to clean up
    // resources on deletion.
    LibvirtClusterFinalizer = "libvirtcluster.infrastructure.cluster.x-k8s.io/finalizer"
)

// -----------------------------------------------------------------------------
// LibvirtClusterSpec
// -----------------------------------------------------------------------------

// LibvirtClusterSpec defines the desired state of LibvirtCluster.
// +kubebuilder:validation:Optional
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="URI",type=string,JSONPath=`.spec.uri`
// +kubebuilder:printcolumn:name="Network",type=string,JSONPath=`.spec.network`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:subresource:status
type LibvirtClusterSpec struct {
    // ControlPlaneEndpoint is the endpoint used to communicate with the
    // control plane.
    // +optional
    ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

    // URI is the Libvirt driver URI (e.g., qemu+tcp://192.168.122.1/system)
    // that the controller will connect to.
    // +kubebuilder:validation:MinLength=1
    // +optional
    URI *string `json:"uri,omitempty"`

    // Network references the Libvirt network to which the cluster’s machines
    // should be attached.
    // +kubebuilder:validation:MinLength=1
    // +optional
    Network *string `json:"network,omitempty"`
}

// -----------------------------------------------------------------------------
// LibvirtClusterStatus
// -----------------------------------------------------------------------------

// LibvirtClusterStatus defines the observed state of LibvirtCluster.
// +kubebuilder:validation:Optional
// +kubebuilder:resource:scope=Namespaced
type LibvirtClusterStatus struct {
    // Ready indicates the cluster infrastructure is ready.
    Ready bool `json:"ready"`

    // Conditions defines the current service state of the LibvirtCluster.
    // +optional
    Conditions clusterv1.Conditions `json:"conditions,omitempty"`

    // FailureMessage indicates terminal errors from the controller.
    // +optional
    FailureMessage *string `json:"failureMessage,omitempty"`
}

// -----------------------------------------------------------------------------
// LibvirtCluster & LibvirtClusterList
// -----------------------------------------------------------------------------

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=libvirtclusters,scope=Namespaced,shortName=lvcluster,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type LibvirtCluster struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   LibvirtClusterSpec   `json:"spec,omitempty"`
    Status LibvirtClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// LibvirtClusterList contains a list of LibvirtCluster.
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:storageversion
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// -----------------------------------------------------------------------------
type LibvirtClusterList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []LibvirtCluster `json:"items"`
}

func init() {
    SchemeBuilder.Register(&LibvirtCluster{}, &LibvirtClusterList{})
}
