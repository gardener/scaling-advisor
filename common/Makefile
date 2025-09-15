# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

MODULE_ROOT     := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT       := $(shell dirname "$(MODULE_ROOT)")
REPO_HACK_DIR   := $(REPO_ROOT)/hack

# include tools targets
include $(REPO_HACK_DIR)/tools.mk

.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

.PHONY: build
build:
	go build ./...

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(REPO_HACK_DIR)/format.sh ./cli ./objutil

.PHONY: check
check: $(GOLANGCI_LINT) format
	@$(REPO_HACK_DIR)/check.sh --golangci-lint-config=../.golangci.yaml ./...

.PHONY: test-unit
test-unit:
	go test -v -race -buildvcs ./...
