// api/v1beta1/constants.go
package v1beta1

// Finalizer strings reused across the provider.
const (
	MachineFinalizer = "libvirtmachine.infrastructure.libvirt.io/finalizer"
	MachineTemplateFinalizer = "libvirtmachinetemplate.infrastructure.libvirt.io/finalizer"
	ClusterFinalizer = "libvirtcluster.infrastructure.libvirt.io/finalizer"
)

// Condition types.
const (
	VMReadyCondition      = "VMReady"
	NetworkReadyCondition = "NetworkReady"
)
