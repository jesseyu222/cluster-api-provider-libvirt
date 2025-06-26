package controller

import (
	"context"
    "fmt"
    libvirt "libvirt.org/go/libvirt"

    infrav1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
    "k8s.io/apimachinery/pkg/runtime"

    "sigs.k8s.io/cluster-api/util"
    "sigs.k8s.io/cluster-api/util/annotations"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
    "strings"
    "time"
)

var machineLog = ctrlLog.Log.WithName("controllers").WithName("LibvirtMachine")
const LibvirtMachineFinalizer = "libvirtmachine.infrastructure.cluster.x-k8s.io/finalizer"
const undefFlags = libvirt.DOMAIN_UNDEFINE_MANAGED_SAVE |
    libvirt.DOMAIN_UNDEFINE_SNAPSHOTS_METADATA |
    libvirt.DOMAIN_UNDEFINE_NVRAM

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

	var infraCluster infrav1.LibvirtCluster
	if err := r.Get(ctx, client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}, &infraCluster); err != nil {
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(&infraMachine, LibvirtMachineFinalizer) {
		controllerutil.AddFinalizer(&infraMachine, LibvirtMachineFinalizer)
		if err := r.Update(ctx, &infraMachine); err != nil {
			return ctrl.Result{}, err
		}
	}

	if !infraMachine.DeletionTimestamp.IsZero() {
		_ = r.deleteLibvirtVM(ctx, &infraMachine, &infraCluster)
		controllerutil.RemoveFinalizer(&infraMachine, LibvirtMachineFinalizer)
		_ = r.Update(ctx, &infraMachine)
		return ctrl.Result{}, nil
	}

	if !cluster.Status.InfrastructureReady || machine.Spec.Bootstrap.DataSecretName == nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// create VM get ProviderID
	if infraMachine.Spec.ProviderID == nil {
		uuid, err := r.createLibvirtVM(ctx, &infraMachine, &infraCluster)
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

func (r *LibvirtMachineReconciler) createLibvirtVM(ctx context.Context, m *infrav1.LibvirtMachine, c *infrav1.LibvirtCluster) (string, error) {
	if c.Spec.URI == nil {
		return "", fmt.Errorf("cluster.spec.uri is nil")
	}
	if m.Spec.Image == nil {
		return "", fmt.Errorf("machine.spec.image is nil")
	}

    // use cluster URI
	uri := strings.TrimSpace(*c.Spec.URI)
	conn, err := libvirt.NewConnect(uri)
    if err != nil {
        return "", fmt.Errorf("libvirt connect (%s): %w", uri, err)
    }
    defer conn.Close()

	if m.Spec.Network == nil {
		nw := "default"
		m.Spec.Network = &nw
	}

	domainXML := fmt.Sprintf(`
<domain type='kvm'>
  <name>%s</name>
  <memory unit='MiB'>%d</memory>
  <vcpu placement='static'>%d</vcpu>
  <os><type arch='x86_64' machine='pc'>hvm</type></os>
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

	dom, err := conn.DomainDefineXML(domainXML)
	if err != nil {
		return "", fmt.Errorf("DomainDefineXML: %w", err)
	}
	if err := dom.Create(); err != nil {
		return "", fmt.Errorf("DomainCreate: %w", err)
	}
	uuid, _ := dom.GetUUIDString()
    return uuid, nil
}

func (r *LibvirtMachineReconciler) deleteLibvirtVM(ctx context.Context, m *infrav1.LibvirtMachine, c *infrav1.LibvirtCluster) error {
	// ------- sanity checks -------
	if c.Spec.URI == nil {
		return fmt.Errorf("cluster.spec.uri is nil")
	}
	if m.Spec.ProviderID == nil && m.Name == "" {
		return fmt.Errorf("cannot determine domain identity (providerID & name empty)")
	}

	// ------- dial libvirtd -------
	conn, err := libvirt.NewConnect(strings.TrimSpace(*c.Spec.URI))
	defer conn.Close()

	// ------- find the domain -----
	var dom *libvirt.Domain
	switch {
	case m.Spec.ProviderID != nil:
		// providerID "libvirt://<uuid>"
		uuidStr := strings.TrimPrefix(*m.Spec.ProviderID, "libvirt://")
		dom, err = conn.LookupDomainByUUIDString(uuidStr)
	case m.Name != "":
		dom, err = conn.LookupDomainByName(m.Name)
	}
	if err != nil {
		if lerr, ok := err.(libvirt.Error); ok && lerr.Code == libvirt.ERR_NO_DOMAIN {
             return nil
         }
		return fmt.Errorf("lookup domain: %w", err)
	}

	// ------- power off (if running) -----
	if state, _, err := dom.GetState(); err == nil && state == libvirt.DOMAIN_RUNNING {
        _ = dom.Destroy()
	}

	// ------- undefine & detach storage -----
	if err := dom.UndefineFlags(undefFlags); err != nil {
        if lerr, ok := err.(libvirt.Error); !ok || lerr.Code != libvirt.ERR_NO_DOMAIN {
            return fmt.Errorf("undefine domain: %w", err)
        }
     
	}
	return nil
}

// func dialInfoFromURI(uri string) (network, address string, err error) {
// 	// empty or "qemu:///system" = UNIX socket
// 	if uri == "" || strings.HasPrefix(uri, "qemu:///") {
// 		return "unix", "/var/run/libvirt/libvirt-sock", nil
// 	}

// 	u, err := url.Parse(uri) // e.g. qemu+tcp://172.31.15.143:16509/system
// 	if err != nil {
// 		return "", "", err
// 	}
// 	host := u.Host
// 	if !strings.Contains(host, ":") {
// 		host += ":16509"
// 	}
// 	return "tcp", host, nil
// }

func (r *LibvirtMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.LibvirtMachine{}).
		Complete(r)
}
