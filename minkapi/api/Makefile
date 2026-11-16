# Change these variables as necessary.

MODULE_ROOT     := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT               := $(realpath $(MODULE_ROOT)/../../)
REPO_HACK_DIR           := $(REPO_ROOT)/hack
$(info MODULE_ROOT is $(MODULE_ROOT))
$(info REPO_ROOT is $(REPO_ROOT))

# include tools targets
include $(REPO_HACK_DIR)/tools.mk

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

.PHONY: no-dirty
no-dirty:
	@test -z "$(shell git status --porcelain)"

## tidy: tidies up go mod
.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

## build: build the planner
.PHONY: build
build:
	go build -v ./...

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(REPO_HACK_DIR)/format.sh ./...

.PHONY: check
check: $(GOLANGCI_LINT) format
	@$(REPO_HACK_DIR)/check.sh --golangci-lint-config=../../.golangci.yaml ./...

## test-unit: run all unit tests with coverage
.PHONY: test-unit
test-unit:
	go test -v -race -buildvcs -cover -coverprofile=${MODULE_ROOT}/coverage.out ./...
	go tool cover -html=${MODULE_ROOT}/coverage.out -o ${MODULE_ROOT}/coverage.html
