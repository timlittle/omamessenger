# OmaMessenger development commands. `make check` runs every gate.
SHELL := /bin/bash
.DEFAULT_GOAL := help

GO ?= go
NODE ?= node
OMARCHY ?= omarchy
OMARCHY_SHELL ?= omarchy-shell
RSYNC ?= rsync
QMLLINT ?= /usr/lib/qt6/bin/qmllint
PLUGIN_ID := io.github.omamessenger
PLUGIN_DIR ?= $(HOME)/.config/omarchy/plugins/$(PLUGIN_ID)
COVERAGE_FILE := build/cover.out

# Third-party tools, pinned and built with the local Go into build/tools/.
GOLANGCI_LINT_VERSION := v2.14.0
GO_TEST_COVERAGE_VERSION := v2.20.0
TOOLS := $(CURDIR)/build/tools
GOLANGCI_LINT := $(TOOLS)/golangci-lint
GO_TEST_COVERAGE := $(TOOLS)/go-test-coverage

.PHONY: help check build build-all install-helper test test-go test-js lint tools validate install-local clean

help: ## Show the development commands
	@awk 'BEGIN {FS = ":.*##"} /^[a-z-]+:.*##/ {printf "  make %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check: build test lint ## Run every gate: build, tests with coverage, lint

build: ## Build the helper for this machine into bin/dev/, which the launcher prefers
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -o bin/dev/oma-messenger-service ./backend

build-all: ## Build the release helpers and SHA256SUMS into build/release/
	./scripts/build-release.sh

install-helper: ## Download and verify the helper release named in helper-version
	./scripts/install-helper.sh

test: test-go test-js ## Run the Go and JavaScript tests

test-go: $(GO_TEST_COVERAGE) ## Run Go tests with the race detector and the coverage gates in .testcoverage.yml
	@mkdir -p $(dir $(COVERAGE_FILE))
	$(GO) test -race -coverprofile=$(COVERAGE_FILE) ./...
	$(GO_TEST_COVERAGE) --config .testcoverage.yml

test-js: ## Run the JavaScript tests with their coverage gate
	$(NODE) --test --experimental-test-coverage --test-coverage-include='ui/lib/**' \
		--test-coverage-lines=95 --test-coverage-branches=90 'tests/unit/**/*.test.cjs'

lint: $(GOLANGCI_LINT) ## Lint Go (golangci-lint, privacy), shell scripts and QML
	$(GOLANGCI_LINT) run ./...
	$(GO) run ./tools/nologcontent ./backend/...
	@if command -v shellcheck >/dev/null; then shellcheck scripts/*.sh bin/oma-messenger-service; \
	else echo "shellcheck not installed; skipping (CI runs it)"; fi
	./scripts/qml-imports.sh
	$(QMLLINT) -I build/qml --max-warnings 0 $$(find ui -name '*.qml')
	git --no-pager diff --check

tools: $(GOLANGCI_LINT) $(GO_TEST_COVERAGE) ## Build the pinned golangci-lint and go-test-coverage

$(GOLANGCI_LINT):
	GOBIN=$(TOOLS) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GO_TEST_COVERAGE):
	GOBIN=$(TOOLS) $(GO) install github.com/vladopajic/go-test-coverage/v2@$(GO_TEST_COVERAGE_VERSION)

validate: ## Validate the plugin with Omarchy
	$(OMARCHY) plugin validate .

install-local: build ## Copy this checkout into the Omarchy plugin directory and enable it
	OMARCHY="$(OMARCHY)" OMARCHY_SHELL="$(OMARCHY_SHELL)" RSYNC="$(RSYNC)" ./scripts/install-local.sh "$(PLUGIN_DIR)"

clean: ## Remove build output
	rm -rf build bin/dev
