package controller

import (
    "context"
    "fmt"
    "time"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"

    clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
    util "sigs.k8s.io/cluster-api/util"

    infrav1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
)

var clusterLog = log.Log.WithName("controllers").WithName("LibvirtCluster")

// LibvirtClusterReconciler reconciles LibvirtCluster objects.
type LibvirtClusterReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

func (r *LibvirtClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := clusterLog.WithValues("libvirtcluster", req.NamespacedName)

    var infraCluster infrav1.LibvirtCluster
    if err := r.Get(ctx, req.NamespacedName, &infraCluster); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Get owning Cluster object (from metadata OwnerReference or label).
    cluster, err := util.GetOwnerCluster(ctx, r.Client, infraCluster.ObjectMeta)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("unable to get owner Cluster: %w", err)
    }
    if cluster == nil {
        log.Info("Owner Cluster not found yet, will requeue.")
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }


    if !hasOwnerReference(&infraCluster, cluster) {
        if err := controllerutil.SetControllerReference(cluster, &infraCluster, r.Scheme); err != nil {
            return ctrl.Result{}, err
        }
        if err := r.Update(ctx, &infraCluster); err != nil {
            return ctrl.Result{}, err
        }
        log.Info("Set ownerReference for LibvirtCluster", "ownerCluster", cluster.Name)

        return ctrl.Result{Requeue: true}, nil
    }

    // Ensure finalizer
    if !controllerutil.ContainsFinalizer(&infraCluster, infrav1.LibvirtClusterFinalizer) {
        controllerutil.AddFinalizer(&infraCluster, infrav1.LibvirtClusterFinalizer)
        if err := r.Update(ctx, &infraCluster); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Deletion handling
    if !infraCluster.DeletionTimestamp.IsZero() {
        log.Info("Deleting cluster resources (stub)")
        _ = r.deleteLibvirtClusterResources(ctx, &infraCluster)
        controllerutil.RemoveFinalizer(&infraCluster, infrav1.LibvirtClusterFinalizer)
        _ = r.Update(ctx, &infraCluster)
        return ctrl.Result{}, nil
    }

    // Bootstrap control-plane endpoint if user did not set one
    if infraCluster.Spec.ControlPlaneEndpoint.Host == "" {
    infraCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
            Host: reserveAvailableIP(),
            Port: 6443,
        }
        if err := r.Update(ctx, &infraCluster); err != nil {
            return ctrl.Result{}, err
        }
        log.Info("Set controlPlaneEndpoint for cluster", "host", infraCluster.Spec.ControlPlaneEndpoint.Host)
    }

    // Mark infra ready
    if !infraCluster.Status.Ready {
        infraCluster.Status.Ready = true
        if err := r.Status().Update(ctx, &infraCluster); err != nil {
            return ctrl.Result{}, err
        }
        log.Info("Marked cluster infra as Ready")
    }

    log.Info("Cluster reconciled", "name", infraCluster.Name)
    return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}


// SetupWithManager registers the controller.
func (r *LibvirtClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&infrav1.LibvirtCluster{}).
        Complete(r)
}

// Helper: You can enhance this to select a free IP dynamically.
func reserveAvailableIP() string { return "192.168.122.250" }

func (r *LibvirtClusterReconciler) deleteLibvirtClusterResources(ctx context.Context, c *infrav1.LibvirtCluster) error {
    // Implement resource cleanup logic here (network, volumes, etc)
    return nil
}
