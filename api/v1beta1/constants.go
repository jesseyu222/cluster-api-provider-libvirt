// api/v1beta1/constants.go
package v1beta1

const (
    // ClusterFinalizer 集群finalizer
    ClusterFinalizer = "libvirtcluster.infrastructure.libvirt.io"
    
    // MachineFinalizer 虚拟机finalizer  
    MachineFinalizer = "libvirtmachine.infrastructure.libvirt.io"
)

// Condition types
const (
    // NetworkReadyCondition 网络就绪条件
    NetworkReadyCondition = "NetworkReady"
    
    // VMReadyCondition 虚拟机就绪条件
    VMReadyCondition = "VMReady"
)