// internal/controller/libvirtcluster_controller.go
package controller

import (
    "context"
    "errors"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"

    clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/handler"
    "sigs.k8s.io/controller-runtime/pkg/manager"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    "sigs.k8s.io/controller-runtime/pkg/source"

    infrastructurev1beta1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
)

var machineLog = ctrl.Log.WithName("controllers").WithName("LibvirtMachine")

// LibvirtClusterReconciler reconciles a LibvirtCluster object
type LibvirtClusterReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=infrastructure.libvirt.io,resources=libvirtclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.libvirt.io,resources=libvirtclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.libvirt.io,resources=libvirtclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// Reconcile LibvirtCluster reconcile逻辑
func (r *LibvirtClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var infraCluster infrastructurev1beta1.LibvirtCluster
    if err := r.Get(ctx, req.NamespacedName, &infraCluster); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    cluster, err := util.GetOwnerCluster(ctx, r.Client, infraCluster.ObjectMeta)
    if err != nil {
        // 無法取得擁有的 Cluster，可能暫時無法處理
        return ctrl.Result{}, err
    }
    if cluster == nil {
        // 若沒有找到 Cluster，跳過
        return ctrl.Result{}, nil
    }

    // 1. 若資源被標記為外部管理則跳過 (此處略)

    // 2. 加入 finalizer
    if !controllerutil.ContainsFinalizer(&infraCluster, infrastructurev1beta1.ClusterFinalizer) {
        controllerutil.AddFinalizer(&infraCluster, infrastructurev1beta1.ClusterFinalizer)
        if err := r.Update(ctx, &infraCluster); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 3. 刪除處理
    if !infraCluster.DeletionTimestamp.IsZero() {
        // 執行刪除清理動作，例如確保所有相關 VM 刪除
        if err := r.deleteLibvirtClusterResources(ctx, &infraCluster); err != nil {
            return ctrl.Result{}, err
        }
        // 移除 finalizer 並更新
        controllerutil.RemoveFinalizer(&infraCluster, infrastructurev1beta1.ClusterFinalizer)
        if err := r.Update(ctx, &infraCluster); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{}, nil
    }

    // 4. 調和正常建立流程
    // 確保Libvirt網路存在 (如需要自訂網路，可在Spec中提供網路名稱)
    err = ensureDefaultNetworkExists()  // pseudo-code 檢查或建立 default 虛擬網路
    if err != nil {
        r.Recorder.Eventf(&infraCluster, corev1.EventTypeWarning, "NetworkError", "Failed to ensure network: %v", err)
        return ctrl.Result{}, err
    }

    // 如果尚未設定 ControlPlaneEndpoint，先嘗試設定
    if infraCluster.Spec.ControlPlaneEndpoint.IsZero() {
        // 選擇或保留一個IP，例如192.168.122.250
        apiHostIP := reserveAvailableIP()
        infraCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
            Host: apiHostIP,
            Port: 6443,
        }
        if err := r.Update(ctx, &infraCluster); err != nil {
            return ctrl.Result{}, err
        }
        // 注意：也可以將此更新延後與Ready一起patch。視實作便利性而定。
    }

    // 標記基礎架構就緒
    infraCluster.Status.Ready = true
    if err := r.Status().Patch(ctx, &infraCluster, patchHelper); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}

func (r *LibvirtClusterReconciler) reconcileNormal(ctx context.Context, libvirtCluster *infrastructurev1beta1.LibvirtCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 添加finalizer
    if !controllerutil.ContainsFinalizer(libvirtCluster, infrastructurev1beta1.ClusterFinalizer) {
        controllerutil.AddFinalizer(libvirtCluster, infrastructurev1beta1.ClusterFinalizer)
        return ctrl.Result{Requeue: true}, nil
    }

    // 创建libvirt客户端
    libvirtClient, err := libvirt.NewClient(libvirtCluster.Spec.URI)
    if err != nil {
        log.Error(err, "Failed to create libvirt client")
        conditions.MarkFalse(libvirtCluster, infrastructurev1beta1.NetworkReadyCondition, "ClientError", clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: time.Minute}, nil
    }
    defer libvirtClient.Close()

    // 确保网络存在
    if err := libvirtClient.EnsureNetwork(libvirtCluster.Spec.Network.Name, libvirtCluster.Spec.Network.CIDR); err != nil {
        log.Error(err, "Failed to ensure network")
        conditions.MarkFalse(libvirtCluster, infrastructurev1beta1.NetworkReadyCondition, "NetworkError", clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: time.Minute}, nil
    }

    // 标记网络就绪
    conditions.MarkTrue(libvirtCluster, infrastructurev1beta1.NetworkReadyCondition)
    libvirtCluster.Status.NetworkReady = true

    // 设置控制平面端点
    if libvirtCluster.Spec.ControlPlaneEndpoint.Host == "" {
        // 这里可以设置负载均衡器或使用第一个控制平面节点的IP
        libvirtCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
            Host: "192.168.122.10", // 这应该是动态分配的
            Port: 6443,
        }
    }

    // 标记集群就绪
    libvirtCluster.Status.Ready = true
    conditions.MarkTrue(libvirtCluster, clusterv1.ReadyCondition)

    return ctrl.Result{}, nil
}

func (r *LibvirtClusterReconciler) reconcileDelete(ctx context.Context, libvirtCluster *infrastructurev1beta1.LibvirtCluster) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 清理libvirt资源
    libvirtClient, err := libvirt.NewClient(libvirtCluster.Spec.URI)
    if err != nil {
        log.Error(err, "Failed to create libvirt client for cleanup")
        // 即使失败也要移除finalizer，避免卡住
    } else {
        defer libvirtClient.Close()
        // 这里可以添加网络清理逻辑
        log.Info("Cleaned up libvirt resources")
    }

    // 移除finalizer
    controllerutil.RemoveFinalizer(libvirtCluster, infrastructurev1beta1.ClusterFinalizer)

    return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LibvirtClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&infrastructurev1beta1.LibvirtCluster{}).
        Complete(r)
}


func ensureDefaultNetworkExists() error {
    // TODO: 檢查或建立 libvirt default network
    return nil
}

func reserveAvailableIP() string {
    // TODO: 從 libvirt DHCP leases 挑可用 IP；暫時先固定
    return "192.168.122.250"
}

func (r *LibvirtClusterReconciler) deleteLibvirtClusterResources(
    ctx context.Context,
    cluster *infrastructurev1beta1.LibvirtCluster,
) error {
    // TODO: 刪除叢集底下所有 VM / 網路資源
    return nil
}