SHELL := /bin/bash
.DEFAULT_GOAL := help

# Build and test commands must never stop for an interactive pager.
export PAGER := cat
export GIT_PAGER := cat
export SYSTEMD_PAGER := cat

GO ?= go
NODE ?= node
DOCKER ?= docker
RSYNC ?= rsync
OMARCHY ?= omarchy
OMARCHY_SHELL ?= omarchy-shell
PLUGIN_ID := io.github.omamessenger
PLUGIN_DIR ?= $(HOME)/.config/omarchy/plugins/$(PLUGIN_ID)
COVERAGE_FILE ?= build/cover.out
# Pinned third-party tools, built with the local Go into build/tools/ on first use.
GOLANGCI_LINT_VERSION := v2.14.0
GO_TEST_COVERAGE_VERSION := v2.20.0
TOOLS := $(CURDIR)/build/tools
GOLANGCI_LINT := $(TOOLS)/golangci-lint
GO_TEST_COVERAGE := $(TOOLS)/go-test-coverage
GOOS := $(shell $(GO) env GOOS)
GOARCH := $(shell $(GO) env GOARCH)

.PHONY: help build build-all install-helper test test-go test-js docs-check test-unit test-integration coverage lint tools validate install-local status pull clean

help: ## Show available development commands
	@awk 'BEGIN {FS = ":.*##"; print "OmaMessenger development commands:"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  make %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the helper for this machine into bin/dev/ (the launcher prefers it; release binaries stay untouched)
	@test "$(GOOS)" = linux || { echo "OmaMessenger helper builds target Linux (found $(GOOS))." >&2; exit 1; }
	CGO_ENABLED=0 $(GO) build -mod=vendor -trimpath -buildvcs=false -o bin/dev/oma-messenger-service ./backend

build-all: ## Build release helpers for amd64 and arm64 with SHA256SUMS into build/release/
	./scripts/build-release.sh

install-helper: ## Download and verify the helper release pinned in helper-version into ~/.local/share/omamessenger/bin/
	./scripts/install-helper.sh

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
	$(NODE) --test 'tests/unit/**/*.test.cjs'

test-unit: ## Run Go and JavaScript tests without the race detector
	$(GO) test -mod=vendor ./backend/... ./tools/...
	$(NODE) --test 'tests/unit/**/*.test.cjs'

test-integration: ## Run backend integration tests with the race detector
	$(GO) test -mod=vendor -race ./backend/...

coverage: $(GO_TEST_COVERAGE) ## Run Go tests with -race and enforce the per-package coverage gates in .testcoverage.yml
	@mkdir -p $(dir $(COVERAGE_FILE))
	$(GO) test -mod=vendor -race -coverprofile=$(COVERAGE_FILE) ./backend/... ./tools/...
	$(GO_TEST_COVERAGE) --config .testcoverage.yml

tools: $(GOLANGCI_LINT) $(GO_TEST_COVERAGE) ## Build the pinned golangci-lint and go-test-coverage into build/tools/

$(GOLANGCI_LINT):
	GOFLAGS=-mod=mod GOBIN=$(TOOLS) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GO_TEST_COVERAGE):
	GOFLAGS=-mod=mod GOBIN=$(TOOLS) $(GO) install github.com/vladopajic/go-test-coverage/v2@$(GO_TEST_COVERAGE_VERSION)

lint: $(GOLANGCI_LINT) ## golangci-lint (.golangci.yml: layering, size, complexity, gofmt, vet), the privacy check, shell syntax, QML, whitespace
	$(GOLANGCI_LINT) run ./...
	$(GO) run -mod=vendor ./tools/nologcontent ./backend/...
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
