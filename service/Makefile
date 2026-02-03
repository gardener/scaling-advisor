MODULE_ROOT     := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT               := $(shell dirname "$(MODULE_ROOT)")
REPO_HACK_DIR           := $(REPO_ROOT)/hack

# include tools targets
include $(REPO_HACK_DIR)/tools.mk

# Change these variables as necessary.
service_main_pkg_path = ./cmd/splanner
service_binary_name = splanner
ctl_main_pkg_path = ./cmd/splanctl
ctl_binary_name = splanctl

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

.PHONY: no-dirty
no-dirty:
	@test -z "$(shell git status --porcelain)"

## test-unit: run all unit tests
.PHONY: test-unit
test-unit:
	go test -v -buildvcs ./...

## test-unit/cover: run all unit tests and display coverage
.PHONY: test-unit/cover
test-unit/cover:
	go test -v -race -buildvcs -cover -coverprofile=${MODULE_ROOT}/coverage.out ./...
	go tool cover -html=${MODULE_ROOT}/coverage.out -o ${MODULE_ROOT}/coverage.html

## test-integration: run all integration tests
.PHONY: test-integration
test-integration:
	go test -v -buildvcs -tags=integration ./test/integration/...

## tidy: tidies up go mod
.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

## build: build the service
.PHONY: build
build:
	mkdir -p bin
	go build -buildvcs -o=bin/${service_binary_name} ${service_main_pkg_path}
	go build -buildvcs -o=bin/${ctl_binary_name} ${ctl_main_pkg_path}

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(REPO_HACK_DIR)/format.sh ./...

.PHONY: check
check: $(GOLANGCI_LINT) format
	@$(REPO_HACK_DIR)/check.sh --golangci-lint-config=../.golangci.yaml ./...