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
GO_LICENSES_VERSION := v2.0.1
GOVULNCHECK_VERSION := v1.8.0
TOOLS := $(CURDIR)/build/tools
# Go's own build scratch goes under build/, on disk: /tmp is a RAM-backed
# tmpfs here, and go-build directories left by an interrupted run sat in
# memory until reboot.
export GOTMPDIR := $(CURDIR)/build/gotmp
$(shell mkdir -p $(GOTMPDIR))
GOLANGCI_LINT := $(TOOLS)/golangci-lint
GO_TEST_COVERAGE := $(TOOLS)/go-test-coverage
GO_LICENSES := $(TOOLS)/go-licenses
GOVULNCHECK := $(TOOLS)/govulncheck

FAKE_HELPER := build/fake/oma-messenger-service
THIRD_PARTY_NOTICES := build/THIRD_PARTY_NOTICES

# Licenses the helper's dependencies may use. go.mau.fi/libsignal (GPL-3.0,
# pulled in by WhatsApp support) is why release binaries are GPL-3.0 even
# though this repository's own source stays MIT; see docs/decisions.md.
ALLOWED_LICENSES := MIT,BSD-2-Clause,BSD-3-Clause,Apache-2.0,ISC,MPL-2.0,GPL-3.0

# github.com/segmentio/asm (a Telegram dependency) is MIT ("MIT No
# Attribution"); go-licenses' classifier does not recognise that exact
# license text and reports it unclassified rather than guessing, so it is
# excluded from the automated check and license report and covered by hand
# in scripts/third-party-notices.tmpl instead. Everything else the helper
# links goes through go-licenses unmodified.
LICENSE_IGNORE := --ignore github.com/segmentio/asm

# Tests run the helper with fake accounts that send messages. Pointing the
# session bus nowhere makes their notify-send calls fail quietly, so no
# test notification reaches the desktop.
NO_DESKTOP_BUS := DBUS_SESSION_BUS_ADDRESS=unix:path=/nonexistent

.PHONY: help check build build-fake build-all install-helper test test-build test-go vulncheck test-js test-qml demo keys lint license-check third-party-notices tools validate install-local release-check benchmark ci clean

help: ## Show the development commands
	@awk 'BEGIN {FS = ":.*##"} /^[a-z-]+:.*##/ {printf "  make %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check: build test test-qml lint license-check ## Run every gate: build, tests with coverage, QML tests, lint, dependency licenses, then a leak check
	@./scripts/check-leaks.sh

build: ## Build the helper for this machine into bin/dev/, which the launcher prefers
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -o bin/dev/oma-messenger-service ./backend

build-fake: ## Build the test helper, which runs scripted fake accounts, into build/fake/
	CGO_ENABLED=0 $(GO) build -tags fake -trimpath -buildvcs=false -o $(FAKE_HELPER) ./backend

build-all: third-party-notices ## Build the release helpers, SHA256SUMS and license notices into build/release/
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

# Compiles the helper, the test build and every race-instrumented test
# binary, exactly as test-go builds them, without running a test: CI's
# build job runs this once and caches the result, so later jobs reuse the
# compiled packages instead of each recompiling the largest ones.
test-build: ## Compile the helper and all test binaries without running tests
	$(GO) build ./...
	$(GO) build -tags fake ./...
	$(GO) test -race -tags fake -coverprofile=$(GOTMPDIR)/test-build.cover -run '^$$' ./... >/dev/null

vulncheck: $(GOVULNCHECK) ## Report known vulnerabilities in the helper's dependencies that its code reaches
	$(GOVULNCHECK) ./backend/...

test-go: $(GO_TEST_COVERAGE) ## Run Go tests with the race detector and the coverage gates in .testcoverage.yml
	@mkdir -p $(dir $(COVERAGE_FILE))
	$(NO_DESKTOP_BUS) $(GO) test -race -tags fake -coverprofile=$(COVERAGE_FILE) ./...
	$(GO_TEST_COVERAGE) --config .testcoverage.yml

test-js: ## Run the JavaScript tests with their coverage gate
	$(NODE) --test --experimental-test-coverage --test-coverage-include='ui/lib/**' \
		--test-coverage-lines=95 --test-coverage-branches=90 'tests/unit/**/*.test.cjs'

keys: ## Print the effective key bindings (defaults plus keys.conf overrides), for a bug report
	$(NODE) scripts/print-key-bindings.cjs

# Each tests/qml/<Name>/shell.qml runs offscreen in its own root, which holds
# the test, the shared tests/qml/Check.js, the ui/ tree, Omarchy's Commons
# and Ui, and as its helper the test build with fake accounts.
# WAYLAND_DISPLAY and HYPRLAND_INSTANCE_SIGNATURE are unset and the session
# bus points nowhere, so no test can reach the running desktop or its
# notifications. A test whose directory holds a no-dev-build file gets the
# real launcher instead, so the helper starts out not installed; it can
# publish the test helper (OMA_FAKE_HELPER) to OMA_RELEASE_BASE and install it.
# Each run also gets a throwaway XDG_RUNTIME_DIR, deleted afterwards:
# quickshell writes a log folder there per instance and never removes it,
# and the real one is a small tmpfs the desktop needs (a full one crashed
# Hyprland).
test-qml: build-fake ## Run the offscreen QML tests in tests/qml/ against the test helper
	@./scripts/qml-imports.sh >/dev/null
	@status=0; for dir in tests/qml/*/; do \
		name=$$(basename "$$dir"); root=build/qml-tests/$$name; run=$$(mktemp -d); \
		rm -rf "$$root"; mkdir -p "$$root/bin"; cp -R "$$dir". "$$root/"; \
		for link in ui scripts helper-version tests/qml/Check.js tests/qml/Stepper.qml tests/qml/FakeShell.qml; do ln -s "$(CURDIR)/$$link" "$$root/$$(basename $$link)"; done; \
		ln -s "$$(readlink -f build/qml/qs/Commons)" "$$root/Commons"; \
		ln -s "$$(readlink -f build/qml/qs/Ui)" "$$root/Ui"; \
		if [ -e "$$dir/no-dev-build" ]; then ln -s "$(CURDIR)/bin/oma-messenger-service" "$$root/bin/"; \
		else ln -s "$(CURDIR)/$(FAKE_HELPER)" "$$root/bin/oma-messenger-service"; fi; \
		if env -u WAYLAND_DISPLAY -u HYPRLAND_INSTANCE_SIGNATURE $(NO_DESKTOP_BUS) QT_QPA_PLATFORM=offscreen \
			XDG_DATA_HOME="$(CURDIR)/$$root/data" XDG_CONFIG_HOME="$(CURDIR)/$$root/config" XDG_RUNTIME_DIR="$$run" OMA_RELEASE_BASE="file://$(CURDIR)/$$root/release" \
			OMA_FAKE_HELPER="$(CURDIR)/$(FAKE_HELPER)" timeout 60 quickshell -p "$$root" >"$$root/log" 2>&1; then echo "ok   $$name"; \
		else status=1; echo "FAIL $$name"; grep -v "qt.qpa" "$$root/log" | grep -E "FAIL|ERROR" | head -20; fi; \
		rm -rf "$$run"; \
	done; \
	warnings=$$(grep -lEi "TypeError|ReferenceError|binding loop" build/qml-tests/*/log 2>/dev/null); \
	if [ -n "$$warnings" ]; then status=1; echo "FAIL unexpected warnings:"; \
		grep -Ei "TypeError|ReferenceError|binding loop" $$warnings; fi; \
	exit $$status

# DEMO_SCENARIOS are the short recordings docs/demo/ gets, one feature
# each, in README order: tests/demo/shell.qml picks which of them to run
# from OMA_DEMO_SCENARIO.
DEMO_SCENARIOS := list-and-send keyboard-nav palette-search media reply-reaction

# Prepares tests/demo/shell.qml's root the same isolated way test-qml
# prepares each of its roots (see above), then for each name in
# DEMO_SCENARIOS plays that scenario offscreen, saving a PNG per frame
# into the root's frames/ directory: Quickshell resolves a path outside
# its own -p root to a blackhole, so the frames have to land inside it.
# QS_DISABLE_FILE_WATCHER=1 stops it treating its own frames as plugin
# source changing underfoot. OMA_FAKE_DEMO=1 switches the fake helper to
# its small, curated demo seed (backend/internal/connector/fake/demo.go),
# not the fuller fixture every other test uses, so the list stays short
# and the names stay neutral. Each scenario gets its own XDG_DATA_HOME and
# XDG_RUNTIME_DIR, so one run's seeded messages or sockets never leak into
# the next. grabToImage occasionally hands back one frame as RGBA instead
# of RGB (seen on the frame right after an overlay's backdrop first
# covers the columns this hides, still mid-blend); ffmpeg's palette
# filter cannot cope with the pixel format changing mid-stream, so every
# frame is normalized to RGB first. ffmpeg then builds a palette from the
# frames for a small, sharp GIF and reuses it, with sierra2_4a dithering
# so the demo photo's sky and water gradients stay smooth on a 256-colour
# palette instead of banding. A held frame from list-and-send's opening
# list becomes the repository's preview.png.
# Re-run this after a UI change to refresh docs/demo/ and preview.png.
# The demo's HOME holds only tests/demo/theme as Omarchy's current theme
# (the shell reads it from ~/.local/state, not an XDG variable), so the clips
# look the same whatever theme the machine recording them has switched to.
demo: build-fake ## Record the offscreen demo GIFs and rebuild docs/demo/ and preview.png
	@command -v $(FFMPEG) >/dev/null || { echo "demo: $(FFMPEG) is not installed" >&2; exit 1; }
	@./scripts/qml-imports.sh >/dev/null
	mkdir -p docs/demo; \
	root=build/demo-root; rm -rf "$$root"; mkdir -p "$$root/bin"; cp -R tests/demo/. "$$root/"; \
	for link in ui scripts helper-version tests/qml/Check.js; do ln -s "$(CURDIR)/$$link" "$$root/$$(basename $$link)"; done; \
	ln -s "$$(readlink -f build/qml/qs/Commons)" "$$root/Commons"; \
	ln -s "$$(readlink -f build/qml/qs/Ui)" "$$root/Ui"; \
	ln -s "$(CURDIR)/$(FAKE_HELPER)" "$$root/bin/oma-messenger-service"; \
	mkdir -p "$$root/home/.local/state/omarchy/current"; \
	ln -s "$(CURDIR)/$$root/theme" "$$root/home/.local/state/omarchy/current/theme"; \
	for name in $(DEMO_SCENARIOS); do \
		run=$$(mktemp -d); \
		rm -rf "$$root/frames" "$$root/data" "$$root/config" "$$root/release" "$$root/log"; mkdir -p "$$root/frames"; \
		env -u WAYLAND_DISPLAY -u HYPRLAND_INSTANCE_SIGNATURE $(NO_DESKTOP_BUS) QT_QPA_PLATFORM=offscreen QS_DISABLE_FILE_WATCHER=1 \
			OMA_FAKE_DEMO=1 OMA_DEMO_SCENARIO="$$name" HOME="$(CURDIR)/$$root/home" \
			XDG_DATA_HOME="$(CURDIR)/$$root/data" XDG_CONFIG_HOME="$(CURDIR)/$$root/config" XDG_RUNTIME_DIR="$$run" OMA_RELEASE_BASE="file://$(CURDIR)/$$root/release" \
			OMA_FAKE_HELPER="$(CURDIR)/$(FAKE_HELPER)" timeout 90 quickshell -p "$$root" >"$$root/log" 2>&1; \
		recorded=$$?; rm -rf "$$run"; \
		if [ $$recorded -ne 0 ]; then \
			echo "FAIL demo recording ($$name):"; grep -v "qt.qpa" "$$root/log" | tail -20; exit 1; \
		fi; \
		for f in "$$root"/frames/*.png; do \
			$(FFMPEG) -y -loglevel error -i "$$f" -pix_fmt rgb24 "$$f.rgb.png" && mv "$$f.rgb.png" "$$f"; \
		done; \
		$(FFMPEG) -y -framerate 10 -i "$$root/frames/frame-%05d.png" \
			-vf "fps=10,scale=960:-1:flags=lanczos,palettegen" -update 1 -frames:v 1 build/demo-palette.png; \
		$(FFMPEG) -y -framerate 10 -i "$$root/frames/frame-%05d.png" -i build/demo-palette.png \
			-lavfi "fps=10,scale=960:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=sierra2_4a" -loop 0 "docs/demo/$$name.gif"; \
		if [ "$$name" = "list-and-send" ]; then cp "$$(ls "$$root"/frames/*.png | tail -n 1)" preview.png; fi; \
	done; \
	ls -lh docs/demo/*.gif preview.png

lint: $(GOLANGCI_LINT) ## Lint Go (golangci-lint, privacy), shell scripts and QML
	$(GOLANGCI_LINT) run ./...
	$(GO) run ./tools/nologcontent ./backend/...
	@if command -v shellcheck >/dev/null; then shellcheck -x scripts/*.sh bin/oma-messenger-service; \
	else echo "shellcheck not installed; skipping (CI runs it)"; fi
	./scripts/qml-imports.sh
	$(QMLLINT) -I build/qml --max-warnings 0 $$(find ui -name '*.qml')
	git --no-pager diff --check

# go-licenses tells the standard library apart by GOROOT. go.mod pins a
# toolchain that Go downloads into the module cache, so point it at that
# toolchain's GOROOT rather than the one go-licenses was built with.
license-check: $(GO_LICENSES) ## Fail if a helper dependency's license is not on the allow-list
	GOROOT="$$($(GO) env GOROOT)" $(GO_LICENSES) check ./backend --allowed_licenses=$(ALLOWED_LICENSES) $(LICENSE_IGNORE)

third-party-notices: $(GO_LICENSES) ## Generate build/THIRD_PARTY_NOTICES, published with each release
	@mkdir -p $(dir $(THIRD_PARTY_NOTICES))
	GOROOT="$$($(GO) env GOROOT)" $(GO_LICENSES) report ./backend $(LICENSE_IGNORE) --ignore github.com/timlittle/omamessenger \
		--template scripts/third-party-notices.tmpl > $(THIRD_PARTY_NOTICES)

tools: $(GOLANGCI_LINT) $(GO_TEST_COVERAGE) $(GO_LICENSES) $(GOVULNCHECK) ## Build the pinned golangci-lint, go-test-coverage, go-licenses and govulncheck

$(GOLANGCI_LINT):
	GOBIN=$(TOOLS) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GO_TEST_COVERAGE):
	GOBIN=$(TOOLS) $(GO) install github.com/vladopajic/go-test-coverage/v2@$(GO_TEST_COVERAGE_VERSION)

$(GO_LICENSES):
	GOBIN=$(TOOLS) $(GO) install github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION)

$(GOVULNCHECK):
	GOBIN=$(TOOLS) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

validate: ## Validate the plugin files, as staged for install, with Omarchy
	OMARCHY="$(OMARCHY)" ./scripts/install-local.sh --check

install-local: build ## Install this checkout into Omarchy, enable it and restart the shell
	OMARCHY="$(OMARCHY)" OMARCHY_SHELL="$(OMARCHY_SHELL)" RSYNC="$(RSYNC)" ./scripts/install-local.sh "$(PLUGIN_DIR)"

# Runs .github/workflows/ci.yml's jobs locally in Docker through act, so a
# push is only made once CI would pass. ubuntu-latest maps to act's own
# Ubuntu image, close to but not identical with GitHub's runner; the qml
# job already runs in its own archlinux container. The jobs run one after
# the other, not side by side, each capped at CI_MEMORY so a run cannot
# starve the desktop, and every act container is removed afterwards, even
# when the run fails or is interrupted.
CI_MEMORY ?= 6g
# Go builds and tests at most this many packages at once inside a local CI
# job, so its compilers stay within CI_MEMORY.
CI_GO_PARALLEL ?= 1
ci: ## Run the GitHub Actions CI workflow locally in Docker (needs act)
	@command -v act >/dev/null || { echo "ci: act is not installed (pacman -S act)" >&2; exit 1; }
	@trap 'docker rm -f $$(docker ps -aq --filter name=act-CI-) >/dev/null 2>&1' EXIT INT TERM; \
	for job in build test checks qml; do \
		act push -W .github/workflows/ci.yml -j "$$job" --rm --env GOFLAGS=-p=$(CI_GO_PARALLEL) \
			--container-options "--memory=$(CI_MEMORY) --memory-swap=$(CI_MEMORY)" \
			-P ubuntu-latest=catthehacker/ubuntu:act-latest || exit 1; \
	done
	@./scripts/check-leaks.sh

benchmark: ## Measure OmaMessenger's memory and CPU use against Telegram Desktop and WhatsApp Web; writes docs/BENCHMARK.md (not part of make check: restarts omarchy-shell and opens real apps)
	./scripts/run-benchmark.sh

clean: ## Remove build output
	rm -rf build bin/dev
