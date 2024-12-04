#!/bin/bash

# Exit immediately for non zero status
set -e
# Check unset variables
set -u
# Print commands
set -x

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

source "$ROOT/scripts/cluster.sh"
source "$ROOT/scripts/lib.sh"

clusters=("east" "west")
all_clusters=$(kind get clusters 2>&1)
matching_clusters=$(echo "$all_clusters" | grep -c -E "$(printf '%s|' "${clusters[@]}" | sed 's/|$//')" || true)

if [ "$matching_clusters" -eq 0 ]; then
  echo "No required clusters found. Provisioning..."
  provision_kind_clusters "${clusters[@]}"
elif [ "$matching_clusters" -ne "${#clusters[@]}" ]; then
  echo "Partial cluster setup detected. Please clean up the environment and retry."
  echo "Suggested command: kind delete clusters ${clusters[*]}"
  exit 1
fi

USE_LOCAL_IMAGE=${USE_LOCAL_IMAGE:-false}
if [ "$USE_LOCAL_IMAGE" == "true" ]; then
  pids=()
  for cluster in "${clusters[@]}"; do
    upload_image "${cluster}" &
    pids+=($!)
  done

  for pid in "${pids[@]}"; do
    wait "$pid" || echo "Process $pid failed"
  done
fi

echo "Provisioning finished"
