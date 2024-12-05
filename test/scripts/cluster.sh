#!/bin/bash

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

provision_kind_clusters() {
  local clusters=("$@")

  pids=()
  for cluster in "${clusters[@]}"; do
    provision_kind_cluster "${cluster}" &
    pids+=($!)
  done

  for pid in "${pids[@]}"; do
    wait "$pid" || { echo "Failed provisioning kind cluster (pid: $pid)"; exit 1; };
  done

  pids=()
  for cluster in "${clusters[@]}"; do
    retry install_metallb "${cluster}" "${clusters[*]}" &
    pids+=($!)
  done

  for pid in "${pids[@]}"; do
    wait "$pid" || { echo "Failed provisioning kind cluster (pid: $pid)"; exit 1; };
  done
}

provision_kind_cluster() {
  local cluster=$1

  kind create cluster --name "$cluster"
  kind get kubeconfig --name "${cluster}" > $ROOT/${cluster}.kubeconfig
}

# Installs and configures MetalLB on a specified Kubernetes cluster.
#
# Args:
#   cluster (string): The name of the Kubernetes cluster where MetalLB should be installed.
#   clusters (string): A comma-separated list of all region clusters, used to calculate a unique CIDR range for the cluster.
#
# Example:
#   install_metallb "us-east-1" "us-east-1,us-west-1,eu-central-1"
install_metallb() {
  local cluster=$1
  local clusters=$2

  echo "Installing MetalLB for $cluster"

  kubectl --kubeconfig="$ROOT/$cluster.kubeconfig" apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml
  kubectl --kubeconfig="$ROOT/$cluster.kubeconfig" wait -n metallb-system pod --timeout=120s -l app=metallb --for=condition=Ready

  local docker_kind_ipv4_subnet="$(docker inspect kind | jq '.[0].IPAM.Config' -r | jq -r '.[] | select(.Subnet | test("^[0-9]+\\.")) | .Subnet')"
  local cidr=$(python3 "$ROOT/scripts/find_subnets.py" --network "$docker_kind_ipv4_subnet" --region $cluster --regions $clusters)
  local federation_ingress_ip=$(python3 "$ROOT/scripts/find_last_host_ip.py" --network "$cidr")
  export "federation_ingress_ip_$cluster"="$federation_ingress_ip"

  echo '
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: default-pool
  namespace: metallb-system
spec:
  addresses:
  - '"$cidr"'
  avoidBuggyIPs: true
---
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: federation-ingress-pool
  namespace: metallb-system
spec:
  addresses:
  - '"$federation_ingress_ip-$federation_ingress_ip"'
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: default-l2
  namespace: metallb-system
spec:
  ipAddressPools:
  - default-pool
  - federation-ingress-ip
' | kubectl apply --kubeconfig="$ROOT/$cluster.kubeconfig" -f -
}

upload_image() {
  local cluster=$1
  local hub=${2:-${HUB:-quay.io/maistra-dev}}
  local tag=${3:-${TAG:-latest}}

  kind load docker-image --nodes "${cluster}-control-plane" \
        --name "$cluster" \
        ${hub}/federation-controller:${tag}
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  "$@"
fi
