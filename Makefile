SHELL := /bin/bash
.DEFAULT_GOAL := help

# Build and test commands must never stop for an interactive pager.
export PAGER := cat
export GIT_PAGER := cat
export SYSTEMD_PAGER := cat

GO ?= go
GO_CACHE ?= /tmp/oma-go-cache
GO_MODULE_CACHE ?= $(shell $(GO) env GOMODCACHE)
NODE ?= node
DOCKER ?= docker
RSYNC ?= rsync
OMARCHY ?= omarchy
OMARCHY_SHELL ?= omarchy-shell
PLUGIN_ID := io.github.omamessenger
PLUGIN_DIR ?= $(HOME)/.config/omarchy/plugins/$(PLUGIN_ID)
COVERAGE_FILE ?= build/cover.out
GOOS := $(shell $(GO) env GOOS)
GOARCH := $(shell $(GO) env GOARCH)

.PHONY: help build build-all test test-go test-js docs-check test-unit test-integration test-omalint cover-omalint run-omalint coverage lint validate install-local status pull clean

help: ## Show available development commands
	@awk 'BEGIN {FS = ":.*##"; print "OmaMessenger development commands:"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  make %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the helper for this machine into bin/dev/ (the launcher prefers it; release binaries stay untouched)
	@test "$(GOOS)" = linux || { echo "OmaMessenger helper builds target Linux (found $(GOOS))." >&2; exit 1; }
	CGO_ENABLED=0 $(GO) build -mod=vendor -trimpath -buildvcs=false -o bin/dev/oma-messenger-service ./backend

build-all: ## Build bundled Linux amd64 and arm64 helpers
	./scripts/build-release.sh

test: ## Build, then run Go (race + coverage gates), JS, lint and docs checks
	+$(MAKE) build
	+$(MAKE) test-go
	+$(MAKE) test-js
	+$(MAKE) lint
	+$(MAKE) docs-check

docs-check: ## Check docs against the code: generated protocol reference, C3/C9/C10 contracts, links, paths, make targets, flags
	$(GO) test -mod=vendor -count=1 ./tools/docscheck/... ./backend/internal/api/ -run 'TestProtocolDocCurrent|TestContract|TestLinks|TestPaths|TestMakeTargets|TestFlags|TestKeys'

test-go: coverage ## Run all Go tests with the race detector and enforce per-package coverage gates

test-js: ## Run the JavaScript logic tests
	$(NODE) --test tests/unit/*.test.cjs

test-unit: ## Run backend tests and keyboard/launcher logic tests
	$(GO) test -mod=vendor ./backend/...
	$(NODE) --test tests/unit/*.test.cjs
	+$(MAKE) test-omalint

test-omalint: ## Run the Go analyzer unit tests with the project Go toolchain
	GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MODULE_CACHE) $(GO) test -mod=vendor ./tools/omalint/...

cover-omalint: ## Show per-package coverage for the Go analyzer suite
	GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MODULE_CACHE) $(GO) test -mod=vendor -cover ./tools/omalint/...

run-omalint: ## Run the Go analyzers over OMALINT_PACKAGES (currently reports known refactor findings)
	GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MODULE_CACHE) $(GO) run -mod=vendor ./tools/omalint $(OMALINT_PACKAGES)

OMALINT_PACKAGES ?= ./backend/... ./tools/...

test-integration: ## Run backend integration tests with the race detector
	$(GO) test -mod=vendor -race ./backend/...

coverage: ## Run Go tests with -race and enforce the C9 per-package coverage gates (tools/covergate)
	@mkdir -p $(dir $(COVERAGE_FILE))
	$(GO) test -mod=vendor -race -coverprofile=$(COVERAGE_FILE) ./backend/... ./tools/...
	$(GO) run -mod=vendor ./tools/covergate $(COVERAGE_FILE)

lint: ## Check Go formatting, vet, architecture rules (omalint), shell scripts, QML, and patch whitespace
	@test -z "$$(gofmt -l backend tools)" || { echo "Go files are not formatted; run gofmt -w backend tools."; exit 1; }
	$(GO) vet -mod=vendor ./...
	+$(MAKE) run-omalint
	bash -n scripts/*.sh tests/e2e/*.sh
	qmllint -I tests/e2e/mocks Panel.qml Service.qml tests/e2e/shell.qml
	git --no-pager diff --check

validate: ## Validate the plugin manifest with Omarchy
	$(OMARCHY) plugin validate .

install-local: build ## Sync this checkout into the Omarchy plugin directory and enable it
	@test "$(PLUGIN_DIR)" = "$(HOME)/.config/omarchy/plugins/$(PLUGIN_ID)" || { echo "Refusing unexpected plugin path: $(PLUGIN_DIR)" >&2; exit 1; }
	OMARCHY="$(OMARCHY)" OMARCHY_SHELL="$(OMARCHY_SHELL)" RSYNC="$(RSYNC)" ./scripts/install-local.sh "$(PLUGIN_DIR)"

status: ## Show the current Git branch and worktree status
	git status --short --branch

pull: ## Fast-forward from the current branch's upstream
	git pull --ff-only

clean: ## Remove local build and test output
	rm -rf build
