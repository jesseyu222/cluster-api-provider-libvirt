package controller

import (
    "context"
    "fmt"
    "time"

    "github.com/libvirt/libvirt-go"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/controller"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    "sigs.k8s.io/cluster-api/util/annotations"
    "sigs.k8s.io/cluster-api/util"
    infrav1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
)

var machineLog = log.Log.WithName("controllers").WithName("LibvirtMachine")

const testVMNamePrefix = "capi-libvirt-"

// LibvirtMachineReconciler reconciles LibvirtMachine objects.
type LibvirtMachineReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *LibvirtMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := machineLog.WithValues("libvirtmachine", req.NamespacedName)

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

    if !hasOwnerReference(&infraMachine, machine) {
        if err := controllerutil.SetControllerReference(machine, &infraMachine, r.Scheme); err != nil {
            log.Error(err, "set ownerReference failed")
            return ctrl.Result{}, err
        }
        if err := r.Update(ctx, &infraMachine); err != nil {
            log.Error(err, "update for ownerReference failed")
            return ctrl.Result{}, err
        }
        log.Info("Set ownerReference for LibvirtMachine", "ownerMachine", machine.Name)
        return ctrl.Result{Requeue: true}, nil
    }


    // Ensure finalizer
    if !controllerutil.ContainsFinalizer(&infraMachine, infrav1.MachineFinalizer) {
        controllerutil.AddFinalizer(&infraMachine, infrav1.MachineFinalizer)
        if err := r.Update(ctx, &infraMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Deletion
    if !infraMachine.DeletionTimestamp.IsZero() {
        log.Info("Deleting VM...")
        _ = r.deleteLibvirtVM(ctx, &infraMachine)
        controllerutil.RemoveFinalizer(&infraMachine, infrav1.MachineFinalizer)
        _ = r.Update(ctx, &infraMachine)
        return ctrl.Result{}, nil
    }

    // Cluster Infra/Bootstrap Ready?
    if !cluster.Status.InfrastructureReady || machine.Spec.Bootstrap.DataSecretName == nil {
        log.Info("Infra or bootstrap not ready yet, requeueing...")
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }


    vmName := testVMNamePrefix + infraMachine.Name
    if !infraMachine.Status.Ready {
        if err := createLibvirtVM(vmName); err != nil {
            log.Error(err, "failed to create libvirt VM")
            return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
        }
        log.Info("Created libvirt VM", "vmName", vmName)
        infraMachine.Status.Ready = true
        // (IP/ProviderID...)
        _ = r.Status().Update(ctx, &infraMachine)
    }

    return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}


func createLibvirtVM(name string) error {
    conn, err := libvirt.NewConnect("qemu:///system")
    if err != nil {
        return fmt.Errorf("connect libvirt: %w", err)
    }
    defer conn.Close()

    domainXML := fmt.Sprintf(`
<domain type='kvm'>
  <name>%s</name>
  <memory unit='MiB'>512</memory>
  <vcpu>1</vcpu>
  <os>
    <type arch='x86_64'>hvm</type>
  </os>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='/var/lib/libvirt/images/test.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <interface type='network'>
      <source network='default'/>
      <model type='virtio'/>
    </interface>
    <graphics type='vnc' port='-1'/>
  </devices>
</domain>`, name)


    dom, err := conn.LookupDomainByName(name)
    if err == nil {
        dom.Free()
        return nil
    }

    dom, err = conn.DomainDefineXML(domainXML)
    if err != nil {
        return fmt.Errorf("define VM: %w", err)
    }
    defer dom.Free()
    if err := dom.Create(); err != nil {
        return fmt.Errorf("start VM: %w", err)
    }
    return nil
}


func (r *LibvirtMachineReconciler) deleteLibvirtVM(ctx context.Context, m *infrav1.LibvirtMachine) error {
    conn, err := libvirt.NewConnect("qemu:///system")
    if err != nil {
        return err
    }
    defer conn.Close()
    vmName := testVMNamePrefix + m.Name
    dom, err := conn.LookupDomainByName(vmName)
    if err != nil {
        return nil // already gone
    }
    defer dom.Free()
    dom.Destroy()
    dom.Undefine()
    return nil
}

func (r *LibvirtMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&infrav1.LibvirtMachine{}).
        WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
        Complete(r)
}

func hasOwnerReference(obj client.Object, owner client.Object) bool {
    for _, ref := range obj.GetOwnerReferences() {
        if ref.UID == owner.GetUID() {
            return true
        }
    }
    return false
}