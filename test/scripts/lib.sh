#!/bin/bash

function upload_image {
  local cluster=$1
  local hub=${2:-${HUB:-quay.io/maistra-dev}}
  local tag=${3:-${TAG:-latest}}

  kind load docker-image --nodes "${cluster}-control-plane" \
        --name "$cluster" \
        ${hub}/federation-controller:${tag}
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

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  "$@"
fi
