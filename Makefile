# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
HACK_DIR            := $(REPO_ROOT)/hack

TOOLS_DIR := $(HACK_DIR)/tools
include $(HACK_DIR)/tools.mk

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@$(HACK_DIR)/addlicenseheaders.sh
