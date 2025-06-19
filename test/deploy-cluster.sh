# Create cluster
kubectl apply -f cluster.yaml

# Get uid
export CLUSTER_UID=$(kubectl get cluster demo-cluster -n default -o jsonpath='{.metadata.uid}')

# Generate new yaml and apply
envsubst < libvirtcluster.yaml | kubectl apply -f -
envsubst < libvirtmachinetemplate.yaml | kubectl apply -f -
envsubst < kubeadmcontrolplane.yaml | kubectl apply -f -
