#!/bin/bash

retry() {
  local command=$@
  local n=1
  local max=5
  local delay=10
  while true; do
    "$@" && break
    if [[ $n -lt $max ]]; then
      ((n++))
      echo "'$command' failed. Attempt $n/$max:"
      sleep $delay;
    else
      echo "'$command' failed after $n attempts."  >&2
      return 2
    fi
  done
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  "$@"
fi
