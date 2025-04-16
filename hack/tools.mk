# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

TOOLS_BIN_DIR           := $(TOOLS_DIR)/bin
GO_ADD_LICENSE          := $(TOOLS_BIN_DIR)/addlicense

# default tool versions
GO_ADD_LICENSE_VERSION  ?= v1.1.1

.PHONY: clean-tools-bin
clean-tools-bin:
	rm -rf $(TOOLS_BIN_DIR)/*

$(GO_ADD_LICENSE):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/google/addlicense@$(GO_ADD_LICENSE_VERSION)
