# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

TOOLS_DIR               := $(REPO_HACK_DIR)/tools
TOOLS_BIN_DIR           := $(TOOLS_DIR)/bin
CONTROLLER_GEN          := $(TOOLS_BIN_DIR)/controller-gen
GOLANGCI_LINT           := $(TOOLS_BIN_DIR)/golangci-lint
GOIMPORTS_REVISER       := $(TOOLS_BIN_DIR)/goimports-reviser
GO_ADD_LICENSE          := $(TOOLS_BIN_DIR)/addlicense
CRD_REF_DOCS            := $(TOOLS_BIN_DIR)/crd-ref-docs
GOSEC                   := $(TOOLS_BIN_DIR)/gosec

# default tool versions
GOLANGCI_LINT_VERSION     ?= v2.1.1
GOIMPORTS_REVISER_VERSION ?= v3.9.1
GO_ADD_LICENSE_VERSION    ?= v1.1.1
CONTROLLER_GEN_VERSION    ?= $(call version_gomod,sigs.k8s.io/controller-tools)
CRD_REF_DOCS_VERSION      ?= v0.1.0
GOSEC_VERSION             ?= v2.22.8

export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

# Use this function to get the version of a go module from go.mod
version_gomod = $(shell go list -mod=mod -f '{{ .Version }}' -m $(1))

.PHONY: clean-tools-bin
clean-tools-bin:
	rm -rf $(TOOLS_BIN_DIR)/*

$(CONTROLLER_GEN):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_GEN_VERSION}

$(GOLANGCI_LINT):
	@# CGO_ENABLED has to be set to 1 in order for golangci-lint to be able to load plugins
	@# see https://github.com/golangci/golangci-lint/issues/1276
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) CGO_ENABLED=1 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOIMPORTS_REVISER):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/incu6us/goimports-reviser/v3@$(GOIMPORTS_REVISER_VERSION)

$(GO_ADD_LICENSE):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/google/addlicense@$(GO_ADD_LICENSE_VERSION)

$(CRD_REF_DOCS):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/elastic/crd-ref-docs@$(CRD_REF_DOCS_VERSION)

$(GOSEC):
	@GOSEC_VERSION=$(GOSEC_VERSION) bash $(TOOLS_DIR)/install-gosec.sh