package controller

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	libvirt "github.com/digitalocean/go-libvirt"
	infrav1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"

	"encoding/hex" 
)

var machineLog = ctrlLog.Log.WithName("controllers").WithName("LibvirtMachine")

const (
    LibvirtMachineFinalizer = "libvirtmachine.infrastructure.cluster.x-k8s.io/finalizer"
    requeueFast             = 20 * time.Second
)

type LibvirtMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// -----------------------------------------------------------------------------
// RBAC
// -----------------------------------------------------------------------------
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=libvirtclusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch


// -----------------------------------------------------------------------------
// Reconcile
// -----------------------------------------------------------------------------
func (r *LibvirtMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Retrieve InfraMachine -------------------------------------------------
    var infraMachine infrav1.LibvirtMachine
    if err := r.Get(ctx, req.NamespacedName, &infraMachine); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Retrieve parent Machine & Cluster ------------------------------------
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

    // 3. Retrieve InfraCluster -------------------------------------------------
    var infraCluster infrav1.LibvirtCluster
    infraRef := cluster.Spec.InfrastructureRef
	if infraRef == nil || infraRef.Kind != "LibvirtCluster" {
		return ctrl.Result{}, fmt.Errorf("cluster %s has no LibvirtCluster reference", cluster.Name)
	}

	// namespace in the ref can be empty → default to cluster namespace
	ns := infraRef.Namespace
	if ns == "" {
		ns = cluster.Namespace
	}

	if err := r.Get(ctx,
		client.ObjectKey{Name: infraRef.Name, Namespace: ns},
		&infraCluster,
	); err != nil {
		return ctrl.Result{}, err
	}

    // 4. Ensure finalizer ------------------------------------------------------
    if !controllerutil.ContainsFinalizer(&infraMachine, LibvirtMachineFinalizer) {
        controllerutil.AddFinalizer(&infraMachine, LibvirtMachineFinalizer)
        if err := r.Update(ctx, &infraMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 5. Handle deletion ------------------------------------------------------
    if !infraMachine.DeletionTimestamp.IsZero() {
        _ = r.deleteLibvirtVM(ctx, &infraMachine, &infraCluster)
        controllerutil.RemoveFinalizer(&infraMachine, LibvirtMachineFinalizer)
        _ = r.Update(ctx, &infraMachine)
        return ctrl.Result{}, nil
    }

    // 6. Wait until the parent Cluster is ready -------------------------------
    if !cluster.Status.InfrastructureReady || machine.Spec.Bootstrap.DataSecretName == nil {
        return ctrl.Result{RequeueAfter: requeueFast}, nil
    }

    // 7. Create VM when ProviderID is not yet set -----------------------------
    if infraMachine.Spec.ProviderID == nil {
        uuid, err := r.createLibvirtVM(ctx, &infraMachine, &infraCluster)
        if err != nil {
            machineLog.Error(err, "create VM failed")
            return ctrl.Result{RequeueAfter: 30 * time.Second}, err
        }
        providerID := fmt.Sprintf("libvirt://%s", uuid)
        infraMachine.Spec.ProviderID = &providerID
        if err := r.Update(ctx, &infraMachine); err != nil {
            return ctrl.Result{}, err
        }
        // Immediately requeue: enter the "wait for VM to boot" phase.
        return ctrl.Result{RequeueAfter: requeueFast}, nil
    }

    // 8. ProviderID is set, but Ready is still false --------------------------
    if !infraMachine.Status.Ready {
        running, err := r.isDomainRunning(ctx, &infraMachine, &infraCluster)
        if err != nil {
            machineLog.Error(err, "check domain state failed")
            return ctrl.Result{RequeueAfter: requeueFast}, nil
        }
        if !running {
            return ctrl.Result{RequeueAfter: requeueFast}, nil
        }
        // Optional: verify cloud‑init completion here.

        // 8.1 Mark Ready -------------------------------------------------------
        orig := infraMachine.DeepCopy()
        infraMachine.Status.Ready = true
        if err := r.Status().Patch(ctx, &infraMachine, client.MergeFrom(orig)); err != nil {
            return ctrl.Result{RequeueAfter: requeueFast}, err
        }
        return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
    }

    // 9. Everything is ready – keep a long heartbeat ---------------------------
    return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// -----------------------------------------------------------------------------
// create VM
// -----------------------------------------------------------------------------
func (r *LibvirtMachineReconciler) createLibvirtVM(
	_ context.Context,
	m *infrav1.LibvirtMachine,
	c *infrav1.LibvirtCluster) (string, error) {

	if c.Spec.URI == nil {
		return "", fmt.Errorf("cluster.spec.uri is nil")
	}
	if m.Spec.Image == nil {
		return "", fmt.Errorf("machine.spec.image is nil")
	}

	netw, addr, err := dialInfoFromURI(strings.TrimSpace(*c.Spec.URI))
	if err != nil {
		return "", err
	}
	conn, err := net.DialTimeout(netw, addr, 5*time.Second)
	if err != nil {
		return "", err
	}
	l := libvirt.New(conn)
	if err := l.Connect(); err != nil {
		return "", err
	}
	defer l.Disconnect()

	if m.Spec.Network == nil {
		def := "default"
		m.Spec.Network = &def
	}

	xml := fmt.Sprintf(`
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

	dom, err := l.DomainDefineXML(xml)
	if err != nil {
		return "", err
	}
	if err := l.DomainCreate(dom); err != nil {
		return "", err
	}

	uuid := fmt.Sprintf("%x", dom.UUID[:]) // 32-hex, no dashes
	return uuid, nil
}

// -----------------------------------------------------------------------------
// delete VM（idempotent）
// -----------------------------------------------------------------------------
const undefFlags = libvirt.DomainUndefineManagedSave |
	libvirt.DomainUndefineSnapshotsMetadata |
	libvirt.DomainUndefineNvram

func (r *LibvirtMachineReconciler) deleteLibvirtVM(_ context.Context, m *infrav1.LibvirtMachine, c *infrav1.LibvirtCluster) error {
	if c.Spec.URI == nil {
		return fmt.Errorf("cluster.spec.uri is nil")
	}
	if m.Spec.ProviderID == nil && m.Name == "" {
		return fmt.Errorf("no identity (providerID & name empty)")
	}

	netw, addr, err := dialInfoFromURI(strings.TrimSpace(*c.Spec.URI))
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout(netw, addr, 5*time.Second)
	if err != nil {
		return err
	}
	l := libvirt.New(conn)
	if err := l.Connect(); err != nil {
		return err
	}
	defer l.Disconnect()

	var dom libvirt.Domain
	switch {
	case m.Spec.ProviderID != nil:
		uuidStr := strings.TrimPrefix(*m.Spec.ProviderID, "libvirt://")
		uuidBytes, err2 := uuidStringToBytes(uuidStr)
		if err2 != nil {
			return err2
		}
		dom, err = l.DomainLookupByUUID(uuidBytes)
	default:
		dom, err = l.DomainLookupByName(m.Name)
	}
	if err != nil {
		if strings.Contains(err.Error(), "domain not found") {
			return nil
		}
		return err
	}

	state, _, _ := l.DomainGetState(dom, 0)
	if libvirt.DomainState(state) == libvirt.DomainRunning {
		_ = l.DomainDestroy(dom)
	}
	if err := l.DomainUndefineFlags(dom, undefFlags); err != nil &&
		!strings.Contains(err.Error(), "domain not found") {
		return err
	}
	return nil
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------
func dialInfoFromURI(uri string) (network, address string, err error) {
	if uri == "" || strings.HasPrefix(uri, "qemu:///") {
		return "unix", "/var/run/libvirt/libvirt-sock", nil
	}
	u, err := url.Parse(uri) // qemu+tcp://10.0.0.1:16509/system
	if err != nil {
		return "", "", err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":16509"
	}
	return "tcp", host, nil
}

func uuidStringToBytes(s string) ([16]byte, error) {
    var out [16]byte
    s = strings.ReplaceAll(s, "-", "")
    if len(s) != 32 {
        return out, fmt.Errorf("invalid UUID %q", s)
    }
    b, err := hex.DecodeString(s)
    if err != nil {
        return out, err
    }
    copy(out[:], b)
    return out, nil
}

// isDomainRunning returns true when the libvirt domain backing this LibvirtMachine
// is in the Running state.
// isDomainRunning checks whether the VM backing this LibvirtMachine
// has reached the Running state.
func (r *LibvirtMachineReconciler) isDomainRunning(
	ctx context.Context,
	m *infrav1.LibvirtMachine,
	c *infrav1.LibvirtCluster) (bool, error) {

	if c.Spec.URI == nil {
		return false, fmt.Errorf("cluster.spec.uri is nil")
	}
	netw, addr, err := dialInfoFromURI(strings.TrimSpace(*c.Spec.URI))
	if err != nil {
		return false, err
	}
	dialTimeout := 5 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl); left < dialTimeout {
			dialTimeout = left
		}
	}
	conn, err := net.DialTimeout(netw, addr, dialTimeout)
	if err != nil {
		return false, err
	}
	l := libvirt.New(conn)
	if err := l.Connect(); err != nil {
		return false, err
	}
	defer l.Disconnect()

	var dom libvirt.Domain
	switch {
	case m.Spec.ProviderID != nil:
		raw, err2 := uuidStringToBytes(strings.TrimPrefix(*m.Spec.ProviderID, "libvirt://"))
		if err2 != nil {
			return false, err2
		}
		dom, err = l.DomainLookupByUUID(raw)
	default:
		dom, err = l.DomainLookupByName(m.Name)
	}
	if err != nil {
		if strings.Contains(err.Error(), "domain not found") {
			return false, nil
		}
		return false, err
	}

	state, _, err := l.DomainGetState(dom, 0)
	if err != nil {
		return false, err
	}
	return libvirt.DomainState(state) == libvirt.DomainRunning, nil
}


func (r *LibvirtMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.LibvirtMachine{}).
		Complete(r)
}

