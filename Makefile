# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_HACK_DIR       := $(REPO_ROOT)/hack
SUBMODULES          := api common minkapi service operator tools

include $(REPO_HACK_DIR)/tools.mk

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@$(REPO_HACK_DIR)/addlicenseheaders.sh

.PHONY: tidy
tidy:
	@for dir in $(SUBMODULES); do \
	  $(MAKE) -C $$dir tidy; \
	done

.PHONY: build
build:
	@for dir in $(SUBMODULES); do \
	  $(MAKE) -C $$dir build; \
	done

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@for dir in $(SUBMODULES); do \
	  $(MAKE) -C $$dir format; \
	done

.PHONY: check
check: $(GOLANGCI_LINT) format
	@for dir in $(SUBMODULES); do \
	  $(MAKE) -C $$dir check; \
	done

.PHONY: test-unit
test-unit:
	@for dir in $(SUBMODULES); do \
	  $(MAKE) -C $$dir test-unit; \
	done
