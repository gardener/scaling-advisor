# Change these variables as necessary.

MODULE_ROOT     := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT               := $(shell dirname "$(MODULE_ROOT)")
REPO_HACK_DIR           := $(REPO_ROOT)/hack

main_pkg_path = .
binary_name = minkapi

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
	mkdir -p bin
    # Include additional build steps, like main_package_pathor Tailwind compilation here...
	go build -buildvcs -o=bin/${service_binary_name} ${service_main_pkg_path}

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(REPO_HACK_DIR)/format.sh ./...

.PHONY: check
check: $(GOLANGCI_LINT) format
	@$(REPO_HACK_DIR)/check.sh --golangci-lint-config=../.golangci.yaml ./...

## test-unit: run all unit tests with coverage
.PHONY: test-unit
test-unit:
	go test -v -race -buildvcs -cover -coverprofile=${MODULE_ROOT}/coverage.out ./...
	go tool cover -html=${MODULE_ROOT}/coverage.out -o ${MODULE_ROOT}/coverage.html
