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

sed "s/{{.clusterName}}/west/g" $ROOT/testdata/istio.yaml > $ROOT/testdata/istio-west.yaml
istioctl manifest generate -f $ROOT/testdata/istio-west.yaml > $ROOT/testdata/istio-west-manifests.yaml
sed "s/{{.clusterName}}/east/g" $ROOT/testdata/istio.yaml > $ROOT/testdata/istio-east.yaml
istioctl manifest generate -f $ROOT/testdata/istio-east.yaml > $ROOT/testdata/istio-east-manifests.yaml
