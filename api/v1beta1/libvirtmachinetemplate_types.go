/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=libvirtmachinetemplates,scope=Namespaced,categories=cluster-api
type LibvirtMachineTemplate struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec LibvirtMachineTemplateSpec `json:"spec,omitempty"`
}

type LibvirtMachineTemplateSpec struct {
    Template LibvirtMachineTemplateResource `json:"template"`
}

// LibvirtMachineTemplateResource defines the spec for the MachineTemplate's Machine.
type LibvirtMachineTemplateResource struct {
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec LibvirtMachineSpec `json:"spec,omitempty"`
}


// +kubebuilder:object:root=true
type LibvirtMachineTemplateList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []LibvirtMachineTemplate `json:"items"`
}