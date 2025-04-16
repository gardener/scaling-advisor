#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e

GOLANGCI_LINT_CONFIG_FILE=""

for arg in "$@"; do
  case $arg in
    --golangci-lint-config=*)
    GOLANGCI_LINT_CONFIG_FILE="-c ${arg#*=}"
    shift
    ;;
  esac
done

echo "> Check"

echo "Executing golangci-lint"
golangci-lint run $GOLANGCI_LINT_CONFIG_FILE --timeout 10m "$@"

echo "Checking Go version"
for module_go_version in $(go list -f {{.GoVersion}} -m)
do
  if ! [[ $module_go_version =~ ^[0-9]+\.[0-9]+\.0$ ]]; then
    echo "Go version is invalid, please adhere to x.y.0 version"
    echo "See https://github.com/gardener/etcd-druid/pull/925"
    exit 1
  fi
done

echo "All checks successful"
