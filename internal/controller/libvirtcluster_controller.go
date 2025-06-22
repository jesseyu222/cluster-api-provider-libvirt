// internal/controller/libvirtcluster_controller.go
package controller

import (
    context "context"
    time "time"

    runtime "k8s.io/apimachinery/pkg/runtime"

    clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
    util "sigs.k8s.io/cluster-api/util"

    ctrl "sigs.k8s.io/controller-runtime"
    controllerutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    client "sigs.k8s.io/controller-runtime/pkg/client"
    ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"

    infrav1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
)

var clusterLog = ctrlLog.Log.WithName("controllers").WithName("LibvirtCluster")

// LibvirtClusterReconciler reconciles LibvirtCluster objects.
type LibvirtClusterReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// Reconcile contains the core reconciliation loop.
func (r *LibvirtClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var infraCluster infrav1.LibvirtCluster
    if err := r.Get(ctx, req.NamespacedName, &infraCluster); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    cluster, err := util.GetOwnerCluster(ctx, r.Client, infraCluster.ObjectMeta)
    if err != nil || cluster == nil {
        return ctrl.Result{}, err
    }

    // Finalizer ensure
    if !controllerutil.ContainsFinalizer(&infraCluster, infrav1.ClusterFinalizer) {
        controllerutil.AddFinalizer(&infraCluster, infrav1.ClusterFinalizer)
        if err := r.Update(ctx, &infraCluster); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Deletion handling
    if !infraCluster.DeletionTimestamp.IsZero() {
        _ = r.deleteLibvirtClusterResources(ctx, &infraCluster)
        controllerutil.RemoveFinalizer(&infraCluster, infrav1.ClusterFinalizer)
        _ = r.Update(ctx, &infraCluster)
        return ctrl.Result{}, nil
    }

    // Bootstrap control-plane endpoint if user did not set one.
    if infraCluster.Spec.ControlPlaneEndpoint.IsZero() {
        infraCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
            Host: reserveAvailableIP(),
            Port: 6443,
        }
        // Update Spec
        if err := r.Update(ctx, &infraCluster); err != nil {
            clusterLog.Error(err, "failed to update infraCluster Spec")
            return ctrl.Result{}, err
        }
        // Re-get for correct resourceVersion
        if err := r.Get(ctx, req.NamespacedName, &infraCluster); err != nil {
            clusterLog.Error(err, "failed to re-get infraCluster after Spec update")
            return ctrl.Result{}, err
        }
    }

    // Mark infra ready
    infraCluster.Status.Ready = true
    if err := r.Status().Update(ctx, &infraCluster); err != nil {
        clusterLog.Error(err, "failed to update infraCluster Status")
        return ctrl.Result{}, err
    }

    clusterLog.Info("cluster reconciled", "name", infraCluster.Name)
    return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

// SetupWithManager registers the controller.
func (r *LibvirtClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&infrav1.LibvirtCluster{}).
        Complete(r)
}

//---------------------------------- helpers ----------------------------------

func reserveAvailableIP() string { return "192.168.122.250" }

func (r *LibvirtClusterReconciler) deleteLibvirtClusterResources(ctx context.Context, c *infrav1.LibvirtCluster) error {
    return nil
}
