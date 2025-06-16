// internal/controller/libvirtmachine_controller.go
package controller

import (
    context "context"
    time "time"

    runtime "k8s.io/apimachinery/pkg/runtime"

    ctrl "sigs.k8s.io/controller-runtime"
    controller "sigs.k8s.io/controller-runtime/pkg/controller"
    controllerutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    client "sigs.k8s.io/controller-runtime/pkg/client"
    ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"

    annotations "sigs.k8s.io/cluster-api/util/annotations"
    util "sigs.k8s.io/cluster-api/util"


    infrav1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
)

var machineLog = ctrlLog.Log.WithName("controllers").WithName("LibvirtMachine")

// LibvirtMachineReconciler reconciles LibvirtMachine objects.
type LibvirtMachineReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.libvirt.io,resources=libvirtmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.libvirt.io,resources=libvirtmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch

// Reconcile performs reconciliation for a LibvirtMachine.
func (r *LibvirtMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var infraMachine infrav1.LibvirtMachine
    if err := r.Get(ctx, req.NamespacedName, &infraMachine); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Get owning Machine and Cluster.
    machine, err := util.GetOwnerMachine(ctx, r.Client, infraMachine.ObjectMeta)
    if err != nil || machine == nil {
        return ctrl.Result{}, err
    }
    cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
    if err != nil || cluster == nil {
        return ctrl.Result{}, err
    }

    // Pause check.
    if annotations.IsPaused(cluster, &infraMachine) {
        return ctrl.Result{}, nil
    }

    // Ensure finalizer.
    if !controllerutil.ContainsFinalizer(&infraMachine, infrav1.MachineFinalizer) {
        controllerutil.AddFinalizer(&infraMachine, infrav1.MachineFinalizer)
        if err := r.Update(ctx, &infraMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Handle deletion.
    if !infraMachine.DeletionTimestamp.IsZero() {
        _ = r.deleteLibvirtVM(ctx, &infraMachine)
        controllerutil.RemoveFinalizer(&infraMachine, infrav1.MachineFinalizer)
        _ = r.Update(ctx, &infraMachine)
        return ctrl.Result{}, nil
    }

    // Wait until cluster infra and bootstrap data are ready.
    if !cluster.Status.InfrastructureReady || machine.Spec.Bootstrap.DataSecretName == nil {
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    // TODO: implement VM create / reconcile logic.
    infraMachine.Status.Ready = true
    _ = r.Status().Update(ctx, &infraMachine)

    return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

// deleteLibvirtVM is a stub for domain deletion.
func (r *LibvirtMachineReconciler) deleteLibvirtVM(ctx context.Context, m *infrav1.LibvirtMachine) error {
    return nil
}

// SetupWithManager registers the controller.
func (r *LibvirtMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&infrav1.LibvirtMachine{}).
        WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
        Complete(r)
}
