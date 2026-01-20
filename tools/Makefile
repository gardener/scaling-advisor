# Change these variables as necessary.

MODULE_ROOT     := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT               := $(shell dirname "$(MODULE_ROOT)")
REPO_HACK_DIR           := $(REPO_ROOT)/hack

scadctl_main_pkg_path = ./cmd/scadctl
scadctl_binary_name = scadctl

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

## test-unit: run all unit tests
.PHONY: test-unit
test-unit:
	go test -v -race -buildvcs ./...

## test-unit/cover: run all unit tests and display coverage
.PHONY: test-unit/cover
test-unit/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

## test-integration: run all integration tests
.PHONY: test-integration
test-integration:
	go test -v -buildvcs -tags=integration ./test/integration/...

## upgradeable: list direct dependencies that have upgrades available
.PHONY: upgradeable
upgradeable:
	@go list -u -f '{{if (and (not (or .Main .Indirect)) .Update)}}{{.Path}}: {{.Version}} -> {{.Update.Version}}{{end}}' -m all

## tidy: tidies up go mod
.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

## build: build the client
.PHONY: build
build:
	mkdir -p bin
	go build -buildvcs -o=bin/${scadctl_binary_name} ${scadctl_main_pkg_path}

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(REPO_HACK_DIR)/format.sh ./...

.PHONY: check
check: $(GOLANGCI_LINT) format
	@$(REPO_HACK_DIR)/check.sh --golangci-lint-config=../.golangci.yaml ./...

## run: run the client
.PHONY: run
run: build
	bin/${binary_name}
