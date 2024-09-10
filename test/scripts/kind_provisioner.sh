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

kind create cluster --name east --config=<<EOF
apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
networking:
  podSubnet: "10.10.0.0/16"
  serviceSubnet: "10.255.10.0/24"
EOF

kind create cluster --name west --config=<<EOF
apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
networking:
  podSubnet: "10.30.0.0/16"
  serviceSubnet: "10.255.30.0/24"
EOF

kind get kubeconfig --name west > $ROOT/west.kubeconfig
kind get kubeconfig --name east > $ROOT/east.kubeconfig

install_metallb_retry west
install_metallb_retry east

for region in east west
do
  sed "s/clusterNamePlaceholder/$region/g" "$ROOT/testdata/manifests/$istio_version/istio.yaml" > "$ROOT/testdata/manifests/$istio_version/istio-$region.yaml"
done
