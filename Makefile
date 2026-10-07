# OmaMessenger development commands. `make check` runs every gate.
SHELL := /bin/bash
.DEFAULT_GOAL := help

GO ?= go
NODE ?= node
OMARCHY ?= omarchy
OMARCHY_SHELL ?= omarchy-shell
RSYNC ?= rsync
QMLLINT ?= /usr/lib/qt6/bin/qmllint
FFMPEG ?= ffmpeg
PLUGIN_ID := io.github.omamessenger
PLUGIN_DIR ?= $(HOME)/.config/omarchy/plugins/$(PLUGIN_ID)
COVERAGE_FILE := build/cover.out

# Third-party tools, pinned and built with the local Go into build/tools/.
GOLANGCI_LINT_VERSION := v2.14.0
GO_TEST_COVERAGE_VERSION := v2.20.0
TOOLS := $(CURDIR)/build/tools
GOLANGCI_LINT := $(TOOLS)/golangci-lint
GO_TEST_COVERAGE := $(TOOLS)/go-test-coverage

FAKE_HELPER := build/fake/oma-messenger-service

# Tests run the helper with fake accounts that send messages. Pointing the
# session bus nowhere makes their notify-send calls fail quietly, so no
# test notification reaches the desktop.
NO_DESKTOP_BUS := DBUS_SESSION_BUS_ADDRESS=unix:path=/nonexistent

.PHONY: help check build build-fake build-all install-helper test test-go test-js test-qml demo lint tools validate install-local release-check clean

help: ## Show the development commands
	@awk 'BEGIN {FS = ":.*##"} /^[a-z-]+:.*##/ {printf "  make %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check: build test test-qml lint ## Run every gate: build, tests with coverage, QML tests, lint

build: ## Build the helper for this machine into bin/dev/, which the launcher prefers
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -o bin/dev/oma-messenger-service ./backend

build-fake: ## Build the test helper, which runs scripted fake accounts, into build/fake/
	CGO_ENABLED=0 $(GO) build -tags fake -trimpath -buildvcs=false -o $(FAKE_HELPER) ./backend

build-all: ## Build the release helpers and SHA256SUMS into build/release/
	./scripts/build-release.sh

# Cross-compiles both release binaries, so it is too slow for `make check`;
# run it by hand before a release, or in CI only for a tag push. It installs
# into a throwaway HOME, never the real one, and cleans up after itself.
release-check: build-all ## Install a real cross-compiled release into a throwaway HOME, end to end
	@tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	mkdir -p "$$tmp/home" "$$tmp/data"; \
	base="file://$(CURDIR)/build/release"; \
	run() { HOME="$$tmp/home" XDG_DATA_HOME="$$tmp/data" OMA_RELEASE_BASE="$$base" ./scripts/install-helper.sh "$$@"; }; \
	if run --status; then echo "release-check: expected --status to fail before install" >&2; exit 1; fi; \
	run; \
	run | grep -q "already installed" || { echo "release-check: second install was not a no-op" >&2; exit 1; }; \
	run --status; \
	test -f "$$tmp/data/applications/io.github.omamessenger.desktop" || { echo "release-check: apps-menu entry missing" >&2; exit 1; }; \
	echo "release-check: installed and verified a real release build, twice, cleanly"

install-helper: ## Download and verify the helper release named in helper-version
	./scripts/install-helper.sh

test: test-go test-js ## Run the Go and JavaScript tests

test-go: $(GO_TEST_COVERAGE) ## Run Go tests with the race detector and the coverage gates in .testcoverage.yml
	@mkdir -p $(dir $(COVERAGE_FILE))
	$(NO_DESKTOP_BUS) $(GO) test -race -tags fake -coverprofile=$(COVERAGE_FILE) ./...
	$(GO_TEST_COVERAGE) --config .testcoverage.yml

test-js: ## Run the JavaScript tests with their coverage gate
	$(NODE) --test --experimental-test-coverage --test-coverage-include='ui/lib/**' \
		--test-coverage-lines=95 --test-coverage-branches=90 'tests/unit/**/*.test.cjs'

# Each tests/qml/<Name>/shell.qml runs offscreen in its own root, which holds
# the test, the shared tests/qml/Check.js, the ui/ tree, Omarchy's Commons
# and Ui, and as its helper the test build with fake accounts.
# WAYLAND_DISPLAY and HYPRLAND_INSTANCE_SIGNATURE are unset and the session
# bus points nowhere, so no test can reach the running desktop or its
# notifications. A test whose directory holds a no-dev-build file gets the
# real launcher instead, so the helper starts out not installed; it can
# publish the test helper (OMA_FAKE_HELPER) to OMA_RELEASE_BASE and install it.
test-qml: build-fake ## Run the offscreen QML tests in tests/qml/ against the test helper
	@./scripts/qml-imports.sh >/dev/null
	@status=0; for dir in tests/qml/*/; do \
		name=$$(basename "$$dir"); root=build/qml-tests/$$name; \
		rm -rf "$$root"; mkdir -p "$$root/bin"; cp -R "$$dir". "$$root/"; \
		for link in ui scripts helper-version tests/qml/Check.js; do ln -s "$(CURDIR)/$$link" "$$root/$$(basename $$link)"; done; \
		ln -s "$$(readlink -f build/qml/qs/Commons)" "$$root/Commons"; \
		ln -s "$$(readlink -f build/qml/qs/Ui)" "$$root/Ui"; \
		if [ -e "$$dir/no-dev-build" ]; then ln -s "$(CURDIR)/bin/oma-messenger-service" "$$root/bin/"; \
		else ln -s "$(CURDIR)/$(FAKE_HELPER)" "$$root/bin/oma-messenger-service"; fi; \
		if env -u WAYLAND_DISPLAY -u HYPRLAND_INSTANCE_SIGNATURE $(NO_DESKTOP_BUS) QT_QPA_PLATFORM=offscreen \
			XDG_DATA_HOME="$(CURDIR)/$$root/data" OMA_RELEASE_BASE="file://$(CURDIR)/$$root/release" \
			OMA_FAKE_HELPER="$(CURDIR)/$(FAKE_HELPER)" timeout 60 quickshell -p "$$root" >"$$root/log" 2>&1; then echo "ok   $$name"; \
		else status=1; echo "FAIL $$name"; grep -v "qt.qpa" "$$root/log" | grep -E "FAIL|ERROR" | head -20; fi; \
	done; \
	warnings=$$(grep -lEi "TypeError|ReferenceError|binding loop" build/qml-tests/*/log 2>/dev/null); \
	if [ -n "$$warnings" ]; then status=1; echo "FAIL unexpected warnings:"; \
		grep -Ei "TypeError|ReferenceError|binding loop" $$warnings; fi; \
	exit $$status

# Prepares tests/demo/shell.qml's root the same isolated way test-qml
# prepares each of its roots (see above), then plays the recorded script
# offscreen, saving a PNG per frame into the root's frames/ directory:
# Quickshell resolves a path outside its own -p root to a blackhole, so
# the frames have to land inside it. QS_DISABLE_FILE_WATCHER=1 stops it
# treating its own frames as plugin source changing underfoot. ffmpeg then
# builds a palette from the frames for a small, sharp GIF and reuses it,
# and a held frame from the opening list becomes the still. Re-run this
# after a UI change to refresh docs/demo.gif and docs/demo.png.
demo: build-fake ## Record the offscreen demo and rebuild docs/demo.gif and docs/demo.png
	@command -v $(FFMPEG) >/dev/null || { echo "demo: $(FFMPEG) is not installed" >&2; exit 1; }
	@./scripts/qml-imports.sh >/dev/null
	root=build/demo-root; \
	rm -rf "$$root"; mkdir -p "$$root/bin" "$$root/frames"; cp -R tests/demo/. "$$root/"; \
	for link in ui scripts helper-version tests/qml/Check.js; do ln -s "$(CURDIR)/$$link" "$$root/$$(basename $$link)"; done; \
	ln -s "$$(readlink -f build/qml/qs/Commons)" "$$root/Commons"; \
	ln -s "$$(readlink -f build/qml/qs/Ui)" "$$root/Ui"; \
	ln -s "$(CURDIR)/$(FAKE_HELPER)" "$$root/bin/oma-messenger-service"; \
	env -u WAYLAND_DISPLAY -u HYPRLAND_INSTANCE_SIGNATURE $(NO_DESKTOP_BUS) QT_QPA_PLATFORM=offscreen QS_DISABLE_FILE_WATCHER=1 \
		XDG_DATA_HOME="$(CURDIR)/$$root/data" OMA_RELEASE_BASE="file://$(CURDIR)/$$root/release" \
		OMA_FAKE_HELPER="$(CURDIR)/$(FAKE_HELPER)" timeout 90 quickshell -p "$$root" >"$$root/log" 2>&1; \
	if [ $$? -ne 0 ]; then \
		echo "FAIL demo recording:"; grep -v "qt.qpa" "$$root/log" | tail -20; exit 1; \
	fi; \
	$(FFMPEG) -y -framerate 10 -i "$$root/frames/frame-%05d.png" \
		-vf "fps=10,scale=960:-1:flags=lanczos,palettegen" -update 1 -frames:v 1 build/demo-palette.png; \
	$(FFMPEG) -y -framerate 10 -i "$$root/frames/frame-%05d.png" -i build/demo-palette.png \
		-lavfi "fps=10,scale=960:-1:flags=lanczos[x];[x][1:v]paletteuse" -loop 0 docs/demo.gif; \
	cp "$$root/frames/frame-00010.png" docs/demo.png; \
	ls -lh docs/demo.gif docs/demo.png

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

validate: ## Validate the plugin files, as staged for install, with Omarchy
	OMARCHY="$(OMARCHY)" ./scripts/install-local.sh --check

install-local: build ## Install this checkout into Omarchy, enable it and restart the shell
	OMARCHY="$(OMARCHY)" OMARCHY_SHELL="$(OMARCHY_SHELL)" RSYNC="$(RSYNC)" ./scripts/install-local.sh "$(PLUGIN_DIR)"

clean: ## Remove build output
	rm -rf build bin/dev
