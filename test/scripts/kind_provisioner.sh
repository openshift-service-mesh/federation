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

metallb_pids=()
install_metallb_retry east &
metallb_pids[0]=$!
install_metallb_retry west
metallb_pids[1]=$!

for pid in ${metallb_pids[*]}; do
  wait $pid
done
