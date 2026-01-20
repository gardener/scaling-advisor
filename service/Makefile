MODULE_ROOT     := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT               := $(shell dirname "$(MODULE_ROOT)")
REPO_HACK_DIR           := $(REPO_ROOT)/hack

# include tools targets
include $(REPO_HACK_DIR)/tools.mk

# Change these variables as necessary.
main_pkg_path = ./cmd/scadsvc
binary_name = scadsvc

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

## build: build the service
.PHONY: build
build:
	mkdir -p bin
    # Include additional build steps, like TypeScript, SCSS or Tailwind compilation here...
	go build -buildvcs -o=bin/${binary_name} ${main_pkg_path}

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(REPO_HACK_DIR)/format.sh ./...

.PHONY: check
check: $(GOLANGCI_LINT) format
	@$(REPO_HACK_DIR)/check.sh --golangci-lint-config=../.golangci.yaml ./...

## run: run the service
.PHONY: run
run: build
	bin/${binary_name}

## run/live: run the service with reloading on file changes
.PHONY: run/live
run/live:
	go run github.com/cosmtrek/air@v1.43.0 \
	    --build.cmd "make build" --build.bin "bin/${binary_name}" --build.delay "100" \
	    --build.exclude_dir "" \
	    --build.include_ext "go, tpl, tmpl, html, css, scss, js, ts, sql, jpeg, jpg, gif, png, bmp, svg, webp, ico" \
	    --misc.clean_on_exit "true"
