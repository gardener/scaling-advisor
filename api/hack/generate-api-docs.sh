#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
MODULE_ROOT="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "${MODULE_ROOT}")"

# make the directory to host the generated API docs
mkdir -p "${REPO_ROOT}/docs/api-reference"

echo "> Generating API docs for ScalingAdvisor APIs..."
crd-ref-docs \
  --source-path "${MODULE_ROOT}" \
  --config "${SCRIPT_DIR}/apidocs/config.yaml" \
  --output-path "${REPO_ROOT}/docs/api-reference/scaling-advisor-api.md" \
  --renderer markdown