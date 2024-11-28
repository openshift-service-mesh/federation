#!/bin/bash

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

function create_kind_cluster {
  cluster=$1
  podSubnet=$2
  svcSubnet=$3

  echo "Creating cluster '$cluster' in podSubnet='$podSubnet' and svcSubnet='$svcSubnet'"

  kind create cluster --name "$cluster" --config=<<EOF
apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
networking:
  podSubnet: "$podSubnet"
  serviceSubnet: "$svcSubnet"
EOF
}

function provision_kind_clusters {
  echo "Creating KinD clusters"
  kind_pids=()
  create_kind_cluster east 10.10.0.0/16 10.255.10.0/24 &
  kind_pids[0]=$!
  create_kind_cluster west 10.30.0.0/16 10.255.30.0/24 &
  kind_pids[1]=$!

  for pid in ${kind_pids[*]}; do
    wait $pid
  done

  kind get kubeconfig --name east > $ROOT/east.kubeconfig
  kind get kubeconfig --name west > $ROOT/west.kubeconfig

  echo "Installing MetalLB"
  metallb_pids=()
  install_metallb_retry east &
  metallb_pids[0]=$!
  install_metallb_retry west &
  metallb_pids[1]=$!

  for pid in ${metallb_pids[*]}; do
    wait $pid
  done
}

function install_metallb_retry {
  retry install_metallb $1
}

function install_metallb() {
  cluster=$1
  kubectl --kubeconfig="$ROOT/$cluster.kubeconfig" apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml
  kubectl --kubeconfig="$ROOT/$cluster.kubeconfig" wait -n metallb-system pod --timeout=120s -l app=metallb --for=condition=Ready

  docker_kind_ipv4_subnet="$(docker inspect kind | jq '.[0].IPAM.Config' -r | jq -r '.[] | select(.Subnet | test("^[0-9]+\\.")) | .Subnet')"
  cidr=$(python3 "$ROOT/scripts/find_smaller_subnets.py" --network "$docker_kind_ipv4_subnet" --region "$cluster")

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
kind: L2Advertisement
metadata:
  name: default-l2
  namespace: metallb-system
spec:
  ipAddressPools:
  - default-pool
' | kubectl apply --kubeconfig="$ROOT/$cluster.kubeconfig" -f -
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  "$@"
fi
