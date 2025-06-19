// api/v1beta1/constants.go
package v1beta1

// Finalizer strings reused across the provider.
const (
	MachineFinalizer = "libvirtmachine.infrastructure.cluster.x-k8s.io/finalizer"
	ClusterFinalizer = "libvirtcluster.infrastructure.cluster.x-k8s.io/finalizer"
)

// Condition types.
const (
	VMReadyCondition      = "VMReady"
	NetworkReadyCondition = "NetworkReady"
)
