#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
REPO_DIR="$(dirname "${SCRIPT_DIR}")"

GOSEC_REPORT="false"
GOSEC_REPORT_PARSE_FLAGS=""
EXCLUDE_DIRS="hack"

function parse_flags() {
  while test $# -gt 1; do
    case "$1" in
      --gosec-report)
        shift;
        GOSEC_REPORT="$1"
        ;;
      --exclude-dirs)
        shift;
        EXCLUDE_DIRS="$1"
        ;;
      *)
        echo "Unknown argument: $1"
        exit 1
        ;;
    esac
    shift
  done
}

parse_flags "$@"

echo "> Running gosec"
gosec --version

if [[ "$GOSEC_REPORT" != "false" ]]; then
  echo "Exporting report to $SCRIPT_DIR/gosec-report.sarif"
  GOSEC_REPORT_PARSE_FLAGS="-track-suppressions -fmt=sarif -out=gosec-report.sarif -stdout"
fi

# exclude generated code, hack directory (where hack scripts reside)
# shellcheck disable=SC2086
gosec -exclude-generated $(echo "$EXCLUDE_DIRS" | awk -v RS=',' '{printf "-exclude-dir %s ", $1}') $GOSEC_REPORT_PARSE_FLAGS ./...