# Cluster API Provider for Libvirt (CAPL) <!-- omit in toc -->

A **Cluster API (CAPI)** infrastructure provider that lets you spin-up complete Kubernetes clusters on a standalone **libvirt/KVM** host or a libvirt network (e.g. QEMU + TCP).  
CAPL turns every libvirt VM into a CAPI‐managed **Machine**, so you can create, scale and upgrade clusters with the same declarative workflow you already know from the public-cloud providers (AWS, Azure, vSphere…).

[Cluster API][capi] • [Libvirt][libvirt]

---

- [Key features](#key-features)  
- [Prerequisites](#prerequisites)  
- [Quick start](#quick-start)  
- [How it works](#how-it-works)  
- [Developer guide](#developer-guide)  
- [Roadmap](#roadmap)  
- [Contributing](#contributing)  
- [License](#license)

---

## Key features
| | |
|---|---|
| **Pure-KVM lab clusters** | Bring-your-own libvirt host, no external cloud needed. |
| **Full CAPI workflow** | `Cluster` ➜ `KubeadmControlPlane` / `MachineDeployment` ➜ automatic HA upgrades. |
| **Image-based provisioning** | Boot from any qcow2/RAW cloud-image (Ubuntu, Fedora Core OS, etc.). |
| **Cloud-Init integration** | CAPI injects kubeadm bootstrap data via user-data. |
| **Bridged / NAT networks** | Specify the libvirt network name for each Machine. |
| **Declarative clean-up** | `kubectl delete cluster` tears down all VMs and storage. |

## Prerequisites
| Component | Tested version |
|-----------|----------------|
| **Kubernetes** (management cluster) | 1.29+ |
| **Cluster API** core/control-plane/bootstrap providers | v1.10+ |
| **Go** (for building) | 1.24+ |
| **Docker / Buildx** | 23.0+ |
| **Libvirt / QEMU** | libvirtd 8.0+, qemu-kvm 7.0+ |

> The management cluster can be a KinD or any existing K8s.  
> All VMs created by CAPL boot as nodes of the **workload cluster**.

---

## Quick start

```bash
# 0) Clone and build/push the controller image
make docker-buildx IMG=<registry>/cluster-api-libvirt-controller:v0.1.0
docker push <registry>/capl-controller:v0.1.0     # or use kind-load

# 1) Install CRDs
make install

# 2) Deploy the controller
make deploy IMG=<registry>/cluster-api-libvirt-controller:v0.1.0

# 3) Create a workload cluster (edit the samples as needed)
kubectl apply -f examples/demo-cluster.yaml

# 4) Watch it come up
clusterctl describe cluster -n demo-cluster demo-cluster
kubectl get clusters -A
kubectl get machines -A
virsh list --all                 # on your libvirt host
