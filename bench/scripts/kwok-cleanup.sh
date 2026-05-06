#!/usr/bin/env bash
set -euo pipefail

clusters=$(kwokctl get clusters 2>/dev/null)
if [[ -z "$clusters" ]]; then
  echo "No kwok clusters found."
  exit 0
fi

while IFS= read -r cluster; do
  echo "Deleting cluster: $cluster"
  kwokctl delete cluster --name "$cluster"
done <<< "$clusters"

echo "Done."
