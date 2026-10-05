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
COVERAGE_MIN ?= 80
COVERAGE_FILE ?= /tmp/oma-messenger-coverage.out
GOOS := $(shell $(GO) env GOOS)
GOARCH := $(shell $(GO) env GOARCH)

.PHONY: help build build-all test test-unit test-integration coverage lint validate install-local status pull clean

help: ## Show available development commands
	@awk 'BEGIN {FS = ":.*##"; print "OmaMessenger development commands:"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  make %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the helper for this machine's architecture
	@test "$(GOOS)" = linux || { echo "OmaMessenger helper builds target Linux (found $(GOOS))." >&2; exit 1; }
	CGO_ENABLED=0 $(GO) build -mod=vendor -trimpath -buildvcs=false -o bin/oma-messenger-service-linux-$(GOARCH) ./backend

build-all: ## Build bundled Linux amd64 and arm64 helpers
	./scripts/build-release.sh

test: ## Build and run backend, keyboard, coverage, and lint checks
	+$(MAKE) build
	+$(MAKE) test-unit
	+$(MAKE) test-integration
	+$(MAKE) coverage
	+$(MAKE) lint

test-unit: ## Run Go store/token tests and keyboard logic tests
	$(GO) test -mod=vendor ./backend -run '^TestStoreAndTokenHelpers$$'
	$(NODE) tests/unit/keyboard.test.cjs

test-integration: ## Run API, SQLite persistence, auth, unread and search integration tests
	$(GO) test -mod=vendor ./backend -run '^TestAPIEndToEndPersistenceSearchUnreadAndAuth$$'

coverage: ## Enforce at least 80% core Go coverage (process bootstrap excluded)
	$(GO) test -mod=vendor -coverprofile=$(COVERAGE_FILE) ./backend
	python3 scripts/check-coverage.py $(COVERAGE_FILE) $(COVERAGE_MIN)

lint: ## Check Go formatting, shell scripts, QML, and patch whitespace
	@test -z "$$(gofmt -l backend/*.go)" || { echo "Go files are not formatted; run gofmt -w backend/*.go."; exit 1; }
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

clean: ## Remove local test coverage output
	rm -f "$(COVERAGE_FILE)"
