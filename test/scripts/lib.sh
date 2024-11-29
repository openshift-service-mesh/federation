#!/bin/bash

retry() {
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
