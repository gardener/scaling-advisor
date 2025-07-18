#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
API_GO_MODULE_ROOT="$(dirname "$SCRIPT_DIR")"

CODE_GEN_DIR=$(go list -m -f '{{.Dir}}' k8s.io/code-generator)
source "${CODE_GEN_DIR}/kube_codegen.sh"

function check_prereq() {
  if ! command -v controller-gen &>/dev/null; then
    echo >&2 "controller-gen is not available, cannot generate deepcopy/runtime.Object for the API types and cannot generate CRDs"
    exit 1
  fi
}

function generate_deepcopy_defaulter() {
  kube::codegen::gen_helpers \
    --boilerplate "${SCRIPT_DIR}/boilerplate.generatego.txt" \
    "${API_GO_MODULE_ROOT}"
}

function generate_crds() {
  local output_dir="${API_GO_MODULE_ROOT}/core/v1alpha1/crds"
  local package="github.com/gardener/scaling-advisor/api/core/v1alpha1"
  local package_path="$(go list -f '{{.Dir}}' "${package}")"

  if [ -z "${package_path}" ]; then
    echo >&2 "Could not locate directory for package: ${package}"
    exit 1
  fi

  if [ -z "${output_dir}" ]; then
    mkdir -p "${output_dir}"
  fi

  # clean all generated crd files
  if ls "${output_dir}/*.yaml" 1> /dev/null 2>&1; then
    rm "${output_dir}/*.yaml"
  fi

  controller-gen crd:allowDangerousTypes=true paths="${package_path}" output:crd:dir="${output_dir}" output:stdout
}

function main() {
  check_prereq
  echo "> Generate deepcopy and defaulting functions..."
  generate_deepcopy_defaulter

  echo "> Generate CRDs..."
  generate_crds
}

main
