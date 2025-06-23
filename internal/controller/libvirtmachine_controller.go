package controller

import (
    context "context"
    fmt "fmt"
    time "time"
    "net"
    runtime "k8s.io/apimachinery/pkg/runtime"
    "github.com/digitalocean/go-libvirt"
    ctrl "sigs.k8s.io/controller-runtime"
    controllerutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    client "sigs.k8s.io/controller-runtime/pkg/client"
    ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
    annotations "sigs.k8s.io/cluster-api/util/annotations"
    util "sigs.k8s.io/cluster-api/util"
    infrav1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
)

var machineLog = ctrlLog.Log.WithName("controllers").WithName("LibvirtMachine")

const LibvirtMachineFinalizer = "libvirtmachine.infrastructure.cluster.x-k8s.io/finalizer"

type LibvirtMachineReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch

func (r *LibvirtMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var infraMachine infrav1.LibvirtMachine
    if err := r.Get(ctx, req.NamespacedName, &infraMachine); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    machine, err := util.GetOwnerMachine(ctx, r.Client, infraMachine.ObjectMeta)
    if err != nil || machine == nil {
        return ctrl.Result{}, err
    }
    cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
    if err != nil || cluster == nil {
        return ctrl.Result{}, err
    }
    if annotations.IsPaused(cluster, &infraMachine) {
        return ctrl.Result{}, nil
    }

    if !controllerutil.ContainsFinalizer(&infraMachine, LibvirtMachineFinalizer) {
        controllerutil.AddFinalizer(&infraMachine, LibvirtMachineFinalizer)
        if err := r.Update(ctx, &infraMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    if !infraMachine.DeletionTimestamp.IsZero() {
        _ = r.deleteLibvirtVM(ctx, &infraMachine)
        controllerutil.RemoveFinalizer(&infraMachine, LibvirtMachineFinalizer)
        _ = r.Update(ctx, &infraMachine)
        return ctrl.Result{}, nil
    }

    if !cluster.Status.InfrastructureReady || machine.Spec.Bootstrap.DataSecretName == nil {
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    // create VM get ProviderID
    if infraMachine.Spec.ProviderID == nil {
        uuid, err := r.createLibvirtVM(ctx, &infraMachine)
        if err != nil {
            machineLog.Error(err, "failed to create Libvirt VM")
            return ctrl.Result{RequeueAfter: 30 * time.Second}, err
        }
        providerID := fmt.Sprintf("libvirt://%s", uuid)
        infraMachine.Spec.ProviderID = &providerID
        if err := r.Update(ctx, &infraMachine); err != nil {
            machineLog.Error(err, "failed to update providerID")
            return ctrl.Result{}, err
        }
    }

    if !infraMachine.Status.Ready {
        infraMachine.Status.Ready = true
        if err := r.Status().Update(ctx, &infraMachine); err != nil {
            machineLog.Error(err, "failed to update machine status")
            return ctrl.Result{}, err
        }
    }

    return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}


func (r *LibvirtMachineReconciler) createLibvirtVM(ctx context.Context, m *infrav1.LibvirtMachine) (string, error) {
    // unix or tcp,tcp here
    conn, err := net.DialTimeout("tcp", "127.0.0.1:16509", 5*time.Second)
    if err != nil {
        return "", fmt.Errorf("dial libvirt failed: %w", err)
    }
    l := libvirt.New(conn)
    if err := l.Connect(); err != nil {
        return "", fmt.Errorf("libvirt connect failed: %w", err)
    }
    defer l.Disconnect()

    // domain XML
    domainXML := fmt.Sprintf(`
<domain type='kvm'>
  <name>%s</name>
  <memory unit='MiB'>%d</memory>
  <vcpu placement='static'>%d</vcpu>
  <os>
    <type arch='x86_64' machine='pc'>hvm</type>
    <boot dev='hd'/>
  </os>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='%s'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <interface type='network'>
      <source network='%s'/>
      <model type='virtio'/>
    </interface>
    <graphics type='vnc' port='-1' autoport='yes'/>
  </devices>
</domain>`, m.Name, m.Spec.MemoryMiB, m.Spec.CPU, *m.Spec.Image, *m.Spec.Network)

    
    dom, err := l.DomainDefineXML(domainXML)
    if err != nil {
        return "", fmt.Errorf("DomainDefineXML failed: %w", err)
    }
    if err := l.DomainCreate(dom); err != nil {
        return "", fmt.Errorf("DomainCreate failed: %w", err)
    }

    // return uuid
    return fmt.Sprintf("%x", dom.UUID), nil
}

func (r *LibvirtMachineReconciler) deleteLibvirtVM(ctx context.Context, m *infrav1.LibvirtMachine) error {
    // TODO: 
    return nil
}

func (r *LibvirtMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&infrav1.LibvirtMachine{}).
        Complete(r)
}
