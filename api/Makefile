# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

API_MODULE_ROOT                 := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT                       := $(shell dirname "$(API_MODULE_ROOT)")
HACK_DIR                        := $(API_MODULE_ROOT)/hack
API_MODULE_TOOLS_DIR            := $(HACK_DIR)/tools
REPO_TOOLS_DIR                  := $(REPO_ROOT)/hack/tools

include $(HACK_DIR)/tools.mk

.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

.PHONY: check-generate
check-generate:
	@$(HACK_DIR)/check-generate.sh "$(API_MODULE_ROOT)"

.PHONY: generate
generate: $(CONTROLLER_GEN) tidy
	@$(HACK_DIR)/generate.sh

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(REPO_ROOT)/hack/format.sh ./core

.PHONY: check
check: $(GOLANGCI_LINT) $(GOIMPORTS) format
	@$(REPO_ROOT)/hack/check.sh --golangci-lint-config=../.golangci.yaml ./...