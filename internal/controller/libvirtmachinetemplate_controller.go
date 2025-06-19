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

package controller

import (
    "context"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"

    infrav1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
)

// LibvirtMachineTemplateReconciler reconciles a LibvirtMachineTemplate object
type LibvirtMachineTemplateReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtmachinetemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtmachinetemplates/status,verbs=get
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtmachinetemplates/finalizers,verbs=update

// Reconcile is a no-op for LibvirtMachineTemplate (template objects are inert, per Cluster API contract)
func (r *LibvirtMachineTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // No reconciliation needed for templates
    return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LibvirtMachineTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&infrav1.LibvirtMachineTemplate{}).
        Named("libvirtmachinetemplate").
        Complete(r)
}
