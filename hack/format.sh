#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

echo "> Format"

for p in "$@" ; do
  goimports-reviser -rm-unused \
   -imports-order "std,company,project,general,blanked,dotted" \
   -format \
   -recursive $p
done
