// internal/controller/libvirtmachine_controller.go
package controller

import (
    "context"
    "encoding/base64"
    "fmt"
    "log"
    "math/rand"
    "os"
    "path/filepath"
    "time"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"

    clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    log "sigs.k8s.io/controller-runtime/pkg/log"

    "sigs.k8s.io/controller-runtime/pkg/controller"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/handler"
    "sigs.k8s.io/controller-runtime/pkg/manager"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    "sigs.k8s.io/controller-runtime/pkg/source"
    "sigs.k8s.io/cluster-api/util"
    "sigs.k8s.io/cluster-api/util/conditions"
    "sigs.k8s.io/cluster-api/util/patch"

    //libvirtgo "github.com/libvirt/libvirt-go"
    libvirtclient "github.com/jesseyu222/cluster-api-provider-libvirt/pkg/libvirt"

    infrastructurev1beta1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
)

// LibvirtMachineReconciler reconciles a LibvirtMachine object
type LibvirtMachineReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=infrastructure.libvirt.io,resources=libvirtmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.libvirt.io,resources=libvirtmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.libvirt.io,resources=libvirtmachines/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *LibvirtMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var infraMachine infrav1.LibvirtMachine
    if err := r.Get(ctx, req.NamespacedName, &infraMachine); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 取得擁有的 Machine 資源（透過 OwnerReference 或標註）
    machine, err := util.GetOwnerMachine(ctx, r.Client, infraMachine.ObjectMeta)
    if err != nil {
        return ctrl.Result{}, err
    }
    if machine == nil {
        return ctrl.Result{}, nil // 無擁有Machine，不處理
    }
    // 取得 Machine 所屬的 Cluster
    cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
    if err != nil {
        return ctrl.Result{}, err
    }
    if cluster == nil {
        return ctrl.Result{}, nil // 無Cluster，不處理
    }

    // 檢查是否 paused
    if annotations.IsPaused(cluster, &infraMachine) {
        return ctrl.Result{}, nil
    }

    // 加入 finalizer
    if !controllerutil.ContainsFinalizer(&infraMachine, infrav1.MachineFinalizer) {
        controllerutil.AddFinalizer(&infraMachine, infrav1.MachineFinalizer)
        if err := r.Update(ctx, &infraMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 刪除處理
    if !infraMachine.DeletionTimestamp.IsZero() {
        // 刪除對應 libvirt VM
        if err := r.deleteLibvirtVM(ctx, &infraMachine); err != nil {
            return ctrl.Result{}, err
        }
        controllerutil.RemoveFinalizer(&infraMachine, infrav1.MachineFinalizer)
        if err := r.Update(ctx, &infraMachine); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{}, nil
    }

    // 若 Cluster 基礎架構尚未就緒，則等待
    if !cluster.Status.InfrastructureReady {
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }
    // 若 Bootstrap 還未有 dataSecretName，則等待
    if machine.Spec.Bootstrap.DataSecretName == nil {
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    // 檢查 VM 是否已存在（例如根據 ProviderID）
    var vmExists bool
    if infraMachine.Spec.ProviderID != nil {
        vmExists = checkVMExists(*infraMachine.Spec.ProviderID)
    }
    // 若不存在則創建
    if !vmExists {
        vmID, vmIP, err := r.createLibvirtVM(ctx, cluster, machine, &infraMachine)
        if err != nil {
            // 創建失敗，記錄事件並重試或返回錯誤
            r.Recorder.Eventf(&infraMachine, corev1.EventTypeWarning, "VMCreateFailed", "Failed to create VM: %v", err)
            return ctrl.Result{}, err
        }
        // 將取得的 ProviderID 填入
        providerID := fmt.Sprintf("libvirt://%s", vmID)
        infraMachine.Spec.ProviderID = &providerID
        // 將首個IP（假設 vmIP 為主要IP）寫入 status.Addresses
        infraMachine.Status.Addresses = []clusterv1.MachineAddress{
            {Type: clusterv1.MachineInternalIP, Address: vmIP},
        }
        // 標記 Ready
        infraMachine.Status.Ready = true

        // 更新 Spec 和 Status
        patchHelper, _ := patch.NewHelper(&infraMachine, r.Client)
        if err := patchHelper.Patch(ctx, &infraMachine); err != nil {
            return ctrl.Result{}, err
        }

        r.Recorder.Eventf(&infraMachine, corev1.EventTypeNormal, "VMCreated", "Created libvirt VM %s with IP %s", vmID, vmIP)
    } else {
        // VM已存在的情況，可檢查狀態變化（本簡化示例不深入實現）
        infraMachine.Status.Ready = true
        _ = r.Status().Update(ctx, &infraMachine)
    }

    return ctrl.Result{}, nil
}


func (r *LibvirtMachineReconciler) reconcileNormal(ctx context.Context, libvirtMachine *infrastructurev1beta1.LibvirtMachine, machine *clusterv1.Machine, cluster *clusterv1.Cluster, libvirtCluster *infrastructurev1beta1.LibvirtCluster) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 添加finalizer
    if !controllerutil.ContainsFinalizer(libvirtMachine, infrastructurev1beta1.MachineFinalizer) {
        controllerutil.AddFinalizer(libvirtMachine, infrastructurev1beta1.MachineFinalizer)
        return ctrl.Result{Requeue: true}, nil
    }

    // 等待bootstrap数据准备就绪
    if machine.Spec.Bootstrap.DataSecretName == nil {
        log.Info("Bootstrap data secret is not yet available")
        conditions.MarkFalse(libvirtMachine, infrastructurev1beta1.VMReadyCondition, "WaitingForBootstrapData", clusterv1.ConditionSeverityInfo, "")
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }

    // 创建libvirt客户端
    libvirtClient, err := libvirtclient.NewClient(libvirtCluster.Spec.URI)
    if err != nil {
        log.Error(err, "Failed to create libvirt client")
        conditions.MarkFalse(libvirtMachine, infrastructurev1beta1.VMReadyCondition, "ClientError", clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: time.Minute}, nil
    }
    defer libvirtClient.Close()

    // 设置虚拟机名称
    if libvirtMachine.Spec.DomainName == "" {
        libvirtMachine.Spec.DomainName = fmt.Sprintf("%s-%s", cluster.Name, machine.Name)
    }

    // 检查虚拟机是否已存在
    existingDomain, err := libvirtClient.GetDomain(libvirtMachine.Spec.DomainName)
    if err == nil {
        // 虚拟机已存在，检查状态
        existingDomain.Free()
        return r.reconcileExistingVM(ctx, libvirtMachine, libvirtClient)
    }

    // 创建新虚拟机
    return r.reconcileNewVM(ctx, libvirtMachine, machine, libvirtCluster, libvirtClient)
}

func (r *LibvirtMachineReconciler) reconcileNewVM(ctx context.Context, libvirtMachine *infrastructurev1beta1.LibvirtMachine, machine *clusterv1.Machine, libvirtCluster *infrastructurev1beta1.LibvirtCluster, libvirtClient *libvirtclient.Client) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 准备磁盘镜像
    diskPath, err := r.prepareDiskImage(ctx, libvirtMachine)
    if err != nil {
        log.Error(err, "Failed to prepare disk image")
        conditions.MarkFalse(libvirtMachine, infrastructurev1beta1.VMReadyCondition, "DiskError", clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: time.Minute}, nil
    }

    // 准备cloud-init数据
    if err := r.prepareCloudInit(ctx, libvirtMachine, machine); err != nil {
        log.Error(err, "Failed to prepare cloud-init data")
        conditions.MarkFalse(libvirtMachine, infrastructurev1beta1.VMReadyCondition, "CloudInitError", clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: time.Minute}, nil
    }

    // 生成MAC地址
    networks := make([]libvirtclient.NetworkConfig, len(libvirtMachine.Spec.Networks))
    for i, net := range libvirtMachine.Spec.Networks {
        mac := net.MAC
        if mac == "" {
            mac = r.generateMAC()
        }
        networks[i] = libvirtclient.NetworkConfig{
            NetworkName: net.NetworkName,
            MAC:         mac,
        }
    }

    // 如果没有指定网络，使用默认网络
    if len(networks) == 0 {
        networks = []libvirtclient.NetworkConfig{
            {
                NetworkName: libvirtCluster.Spec.Network.Name,
                MAC:         r.generateMAC(),
            },
        }
    }

    // 创建虚拟机配置
    domainConfig := libvirtclient.DomainConfig{
        Name:     libvirtMachine.Spec.DomainName,
        Memory:   libvirtMachine.Spec.Memory,
        VCPU:     libvirtMachine.Spec.VCPU,
        DiskPath: diskPath,
        Networks: networks,
    }

    // 创建虚拟机
    domain, err := libvirtClient.CreateDomain(domainConfig)
    if err != nil {
        log.Error(err, "Failed to create domain")
        conditions.MarkFalse(libvirtMachine, infrastructurev1beta1.VMReadyCondition, "CreateError", clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: time.Minute}, nil
    }
    defer domain.Free()

    // 启动虚拟机
    if err := libvirtClient.StartDomain(libvirtMachine.Spec.DomainName); err != nil {
        log.Error(err, "Failed to start domain")
        conditions.MarkFalse(libvirtMachine, infrastructurev1beta1.VMReadyCondition, "StartError", clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: time.Minute}, nil
    }

    // 设置ProviderID
    providerID := fmt.Sprintf("libvirt:///%s", libvirtMachine.Spec.DomainName)
    libvirtMachine.Spec.ProviderID = &providerID

    log.Info("Successfully created and started VM", "domain", libvirtMachine.Spec.DomainName)
    conditions.MarkTrue(libvirtMachine, infrastructurev1beta1.VMReadyCondition)

    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *LibvirtMachineReconciler) reconcileExistingVM(ctx context.Context, libvirtMachine *infrastructurev1beta1.LibvirtMachine, libvirtClient *libvirtclient.Client) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 获取虚拟机状态
    state, err := libvirtClient.GetDomainState(libvirtMachine.Spec.DomainName)
    if err != nil {
        log.Error(err, "Failed to get domain state")
        conditions.MarkFalse(libvirtMachine, infrastructurev1beta1.VMReadyCondition, "StateError", clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: time.Minute}, nil
    }

    // 更新状态
    libvirtMachine.Status.InstanceState = r.libvirtStateToString(state)

    // 如果虚拟机未运行，尝试启动
    if state != libvirt.DOMAIN_RUNNING {
        if err := libvirtClient.StartDomain(libvirtMachine.Spec.DomainName); err != nil {
            log.Error(err, "Failed to start existing domain")
            conditions.MarkFalse(libvirtMachine, infrastructurev1beta1.VMReadyCondition, "StartError", clusterv1.ConditionSeverityError, err.Error())
            return ctrl.Result{RequeueAfter: time.Minute}, nil
        }
    }

    // 获取IP地址
    ips, err := libvirtClient.GetDomainIPs(libvirtMachine.Spec.DomainName)
    if err != nil {
        log.Info("Could not get domain IPs, will retry", "error", err.Error())
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }

    // 更新地址信息
    addresses := make([]clusterv1.MachineAddress, 0, len(ips))
    for _, ip := range ips {
        addresses = append(addresses, clusterv1.MachineAddress{
            Type:    clusterv1.MachineInternalIP,
            Address: ip,
        })
        addresses = append(addresses, clusterv1.MachineAddress{
            Type:    clusterv1.MachineExternalIP,
            Address: ip,
        })
    }
    libvirtMachine.Status.Addresses = addresses

    // 设置ProviderID
    if libvirtMachine.Spec.ProviderID == nil {
        providerID := fmt.Sprintf("libvirt:///%s", libvirtMachine.Spec.DomainName)
        libvirtMachine.Spec.ProviderID = &providerID
    }

    // 标记就绪
    libvirtMachine.Status.Ready = true
    conditions.MarkTrue(libvirtMachine, infrastructurev1beta1.VMReadyCondition)

    return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

func (r *LibvirtMachineReconciler) reconcileDelete(ctx context.Context, libvirtMachine *infrastructurev1beta1.LibvirtMachine, libvirtCluster *infrastructurev1beta1.LibvirtCluster) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 创建libvirt客户端
    libvirtClient, err := libvirtclient.NewClient(libvirtCluster.Spec.URI)
    if err != nil {
        log.Error(err, "Failed to create libvirt client for cleanup")
        // 即使失败也要移除finalizer，避免卡住
    } else {
        defer libvirtClient.Close()

        // 删除虚拟机
        if err := libvirtClient.DeleteDomain(libvirtMachine.Spec.DomainName); err != nil {
            log.Error(err, "Failed to delete domain", "domain", libvirtMachine.Spec.DomainName)
        } else {
            log.Info("Successfully deleted VM", "domain", libvirtMachine.Spec.DomainName)
        }

        // 清理磁盘文件
        r.cleanupDiskImage(libvirtMachine)
    }

    // 移除finalizer
    controllerutil.RemoveFinalizer(libvirtMachine, infrastructurev1beta1.MachineFinalizer)

    return ctrl.Result{}, nil
}

// prepareDiskImage 准备磁盘镜像
func (r *LibvirtMachineReconciler) prepareDiskImage(ctx context.Context, libvirtMachine *infrastructurev1beta1.LibvirtMachine) (string, error) {
    // 创建存储目录
    storageDir := "/var/lib/libvirt/images"
    if err := os.MkdirAll(storageDir, 0755); err != nil {
        return "", fmt.Errorf("failed to create storage directory: %w", err)
    }

    diskPath := filepath.Join(storageDir, fmt.Sprintf("%s.qcow2", libvirtMachine.Spec.DomainName))

    // 如果磁盘已存在，直接返回
    if _, err := os.Stat(diskPath); err == nil {
        return diskPath, nil
    }

    // 如果指定了本地路径，复制文件
    if libvirtMachine.Spec.Image.Path != "" {
        cmd := exec.Command("qemu-img", "create", "-f", "qcow2", "-b", srcPath, destPath)
        output, err := cmd.CombinedOutput()
        if err != nil {
            return fmt.Errorf("failed to create qcow2 image: %s - output: %s", err, string(output))
        }
        return nil
    }

    // 如果指定了URL，下载镜像
    if libvirtMachine.Spec.Image.URL != "" {
        return r.downloadDiskImage(libvirtMachine.Spec.Image.URL, diskPath)
    }

    return "", fmt.Errorf("no image source specified")
}

func (r *LibvirtMachineReconciler) copyDiskImage(srcPath, dstPath string) (string, error) {
    // 这里应该实现磁盘镜像复制逻辑
    // 可以使用qemu-img create -f qcow2 -b srcPath dstPath创建基于基础镜像的镜像
    return dstPath, nil
}

func (r *LibvirtMachineReconciler) downloadDiskImage(url, dstPath string) (string, error) {
    // 这里应该实现镜像下载逻辑
    // 可以使用HTTP客户端下载镜像文件
    return dstPath, nil
}

func (r *LibvirtMachineReconciler) cleanupDiskImage(libvirtMachine *infrastructurev1beta1.LibvirtMachine) {
    diskPath := filepath.Join("/var/lib/libvirt/images", fmt.Sprintf("%s.qcow2", libvirtMachine.Spec.DomainName))
    os.Remove(diskPath)
}

// prepareCloudInit 准备cloud-init数据
func (r *LibvirtMachineReconciler) prepareCloudInit(ctx context.Context, libvirtMachine *infrastructurev1beta1.LibvirtMachine, machine *clusterv1.Machine) error {
    // 获取bootstrap数据
    bootstrapSecret := &corev1.Secret{}
    bootstrapSecretKey := client.ObjectKey{
        Namespace: machine.Namespace,
        Name:      *machine.Spec.Bootstrap.DataSecretName,
    }
    if err := r.Get(ctx, bootstrapSecretKey, bootstrapSecret); err != nil {
        return fmt.Errorf("failed to get bootstrap secret: %w", err)
    }

    userData := string(bootstrapSecret.Data["value"])
    
    // 编码用户数据
    libvirtMachine.Spec.CloudInit = &infrastructurev1beta1.LibvirtCloudInitSpec{
        UserData: base64.StdEncoding.EncodeToString([]byte(userData)),
    }

    return nil
}

// generateMAC 生成随机MAC地址
func (r *LibvirtMachineReconciler) generateMAC() string {
    rand.Seed(time.Now().UnixNano())
	mac := []byte{0x52, 0x54, 0x00, byte(rand.Intn(256)), byte(rand.Intn(256)), byte(rand.Intn(256))}
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

// libvirtStateToString 转换libvirt状态为字符串
func (r *LibvirtMachineReconciler) libvirtStateToString(state libvirt.DomainState) *string {
    var stateStr string
    switch state {
    case libvirt.DOMAIN_RUNNING:
        stateStr = "running"
    case libvirt.DOMAIN_BLOCKED:
        stateStr = "blocked"
    case libvirt.DOMAIN_PAUSED:
        stateStr = "paused"
    case libvirt.DOMAIN_SHUTDOWN:
        stateStr = "shutdown"
    case libvirt.DOMAIN_SHUTOFF:
        stateStr = "shutoff"
    case libvirt.DOMAIN_CRASHED:
        stateStr = "crashed"
    default:
        stateStr = "unknown"
    }
    return &stateStr
}

// SetupWithManager sets up the controller with the Manager.
func (r *LibvirtMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.LibvirtMachine{}).
		Owns(&corev1.Secret{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			handler.EnqueueRequestsFromMapFunc(capicluster.ClusterToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("LibvirtMachine"))),
		).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("LibvirtMachine"))),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		Complete(r)
}
