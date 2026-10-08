#!/usr/bin/env bash
set -euo pipefail

cluster=$(kwokctl get clusters 2>/dev/null | head -1)
if [[ -z "$cluster" ]]; then
  echo "No kwok clusters found. Start one first." >&2
  exit 1
fi

echo "Using cluster: $cluster" >&2
kwokctl get kubeconfig --name "$cluster"
