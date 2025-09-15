# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_HACK_DIR       := $(REPO_ROOT)/hack

include $(REPO_HACK_DIR)/tools.mk

# run-in-submodules iterates over all submodules and invokes the passed in make target.
define run-in-submodules
	@for dir in $$(find . -type f -name go.mod -exec dirname {} \; | sort); do \
		echo "Running 'make $(1)' in $$dir"; \
		$(MAKE) -C $$dir $(1) || exit 1; \
	done
endef

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@$(REPO_HACK_DIR)/addlicenseheaders.sh

.PHONY: tidy
tidy:
	$(call run-in-submodules,tidy)

.PHONY: build
build:
	$(call run-in-submodules,build)

.PHONY: format
format: $(GOIMPORTS_REVISER)
	$(call run-in-submodules,format)

.PHONY: check
check: $(GOLANGCI_LINT) format
	$(call run-in-submodules,check)

.PHONY: test-unit
test-unit:
	$(call run-in-submodules,test-unit)
