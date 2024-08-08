#!/bin/bash

WD=$(dirname "$0")
WD=$(cd "$WD"; pwd)
ROOT=$(dirname "$WD")

function install_metallb_retry {
  retry install_metallb $1
}

function install_metallb() {
  cluster=$1
  kubectl --kubeconfig="$ROOT/$cluster.kubeconfig" apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml
  kubectl --kubeconfig="$ROOT/$cluster.kubeconfig" wait -n metallb-system pod --timeout=120s -l app=metallb --for=condition=Ready

  docker_kind_subnet="$(docker inspect kind | jq '.[0].IPAM.Config[0].Subnet' -r)"
  cidr=$(python3 "$ROOT/scripts/find_smaller_subnets.py" --network "$docker_kind_subnet" --region "$cluster")

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

function retry {
  local n=1
  local max=5
  local delay=5
  while true; do
    "$@" && break
    if [[ $n -lt $max ]]; then
      ((n++))
      echo "Command failed. Attempt $n/$max:"
      sleep $delay;
    else
      echo "The command has failed after $n attempts."  >&2
      return 2
    fi
  done
}
