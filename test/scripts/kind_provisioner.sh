#!/bin/bash

# Exit immediately for non zero status
set -e
# Check unset variables
set -u
# Print commands
set -x

WD=$(dirname "$0")
WD=$(cd "$WD"; pwd)
ROOT=$(dirname "$WD")

istio_version=$1

source "$ROOT/scripts/lib.sh"

found_clusters=0
for name in "east" "west"; do
  if kind get clusters | grep -q "$name"; then
    found_clusters=$((found_clusters+1))
  else
    echo "Not found cluster $name"
  fi
done

if [ "$found_clusters" -eq "2" ]; then
  echo "All clusters were found - skipping clusters provisioning."
  exit 0
elif [ "$found_clusters" -ne "0" ]; then
  echo "Did not find all clusters, but some exist - cleanup environment and run script again."
  exit 1
fi

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

echo "Uploading images"
for cluster_name in "east" "west"; do
  kind load docker-image --nodes "${cluster_name}-control-plane" --name "$cluster_name" quay.io/maistra-dev/federation-controller:test
done

echo "Installing MetalLB"
metallb_pids=()
install_metallb_retry east &
metallb_pids[0]=$!
install_metallb_retry west &
metallb_pids[1]=$!

for pid in ${metallb_pids[*]}; do
  wait $pid
done
