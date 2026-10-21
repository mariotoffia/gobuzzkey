# Makefile for gobuzzkey.
#
# Two gates decide "done" (AGENTS.md):
#   make lint   every static check; one report per checker under reports/
#   make test   unit tests (race) + test timing audit

.PHONY: all build test test-live fuzz lint lint-fix check vulncheck dist release clean tidy install help
.PHONY: audit-test-timings dupl-report goconst-report

# Tool pins. CI installs the same versions (.github/workflows/ci.yml).
GOLANGCI_LINT_VERSION ?= v2.15.0
GO_ARCH_LINT_VERSION  ?= v1.19.0
DUPL_VERSION          ?= v1.1.0
GOCONST_VERSION       ?= v1.10.2
GOVULNCHECK_VERSION   ?= v1.9.0

# Build tools with the module's toolchain (go.mod "toolchain"), not whatever
# go happens to be first on PATH: a tool built with an older Go cannot load
# packages that require Go 1.27.
GO_TOOLCHAIN := $(shell go env GOVERSION)
TOOL_ENV     := GOTOOLCHAIN=$(GO_TOOLCHAIN)

BIN       := gobuzzkey
CMD       := ./cmd/gobuzzkey
DIST      := dist
FUZZTIME  ?= 30s
PLATFORMS ?= darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64

VERSION ?=
CONFIRM ?=

all: check

build: ## Build the gobuzzkey binary into ./gobuzzkey.out
	go build -trimpath -o $(BIN).out $(CMD)

test: audit-test-timings ## Run unit tests with the race detector (live relay test skipped)
	@mkdir -p reports
	@bash -c 'set -o pipefail; go test -race -short -count=1 -timeout 120s ./... 2>&1 | tee reports/test.log'

test-live: ## Run tests/live against GOBUZZKEY_LIVE_ENV (optional GOBUZZKEY_LIVE_CHANNEL)
	@test -n "$$GOBUZZKEY_LIVE_ENV" || { echo "set GOBUZZKEY_LIVE_ENV to an identity file"; exit 2; }
	go test -count=1 -v ./tests/live/...

fuzz: ## Mutate every fuzz target for FUZZTIME each (seed corpora already run in make test)
	@mkdir -p reports
	@bash -c 'set -eo pipefail; : > reports/fuzz.log; \
	for pkg in $$(go list ./...); do \
		for target in $$(go test -list "^Fuzz" "$$pkg" | grep "^Fuzz" || true); do \
			echo "--- $$pkg $$target ---" | tee -a reports/fuzz.log; \
			go test -run "^$$" -fuzz "^$$target$$" -fuzztime $(FUZZTIME) "$$pkg" 2>&1 | tee -a reports/fuzz.log; \
		done; \
	done'

lint: ## Run every static check; writes reports/*
	@mkdir -p reports
	@echo "=== Domain purity ==="
	@bash -c 'set -o pipefail; bash scripts/lint-domain-purity.sh 2>&1 | tee reports/domain-purity.log'
	@echo "=== Architecture lint ==="
	@bash -c 'set -o pipefail; go-arch-lint check --project-path . --output-color=false 2>&1 | tee reports/go-arch-lint.log'
	@go-arch-lint graph --out reports/go-arch-lint-graph.svg >/dev/null
	@echo "=== Planning-reference governance ==="
	@bash -c 'set -o pipefail; bash scripts/lint-planning-refs.sh --self-test 2>&1 | tee reports/planning-refs.log'
	@echo "=== gofmt ==="
	@FILES=$$(gofmt -l $$(git ls-files '*.go' 2>/dev/null || find . -name '*.go')); \
	echo "$$FILES" > reports/gofmt.log; \
	if [ -n "$$FILES" ]; then echo "$$FILES"; echo "Run: make lint-fix"; exit 1; fi
	@echo "=== go vet ==="
	@bash -c 'set -o pipefail; go vet ./... 2>&1 | tee reports/go-vet.log'
	@echo "=== golangci-lint ==="
	@bash -c 'major=$$(golangci-lint version 2>/dev/null | sed -nE "s/.*version v?([0-9]+).*/\1/p" | head -1); \
	if [ "$$major" != "2" ]; then echo "ERROR: golangci-lint v2 required (run: make install)"; exit 1; fi'
	@bash -c 'set -o pipefail; golangci-lint run --timeout=5m ./... 2>&1 | tee reports/golangci.log'
	@echo "=== Duplicate scan (advisory) ==="
	@$(MAKE) --no-print-directory dupl-report
	@echo "=== Repeated literals (advisory) ==="
	@$(MAKE) --no-print-directory goconst-report

lint-fix: ## Format every Go file with gofmt
	gofmt -w $$(git ls-files '*.go')

check: build lint test ## build + lint + test

dupl-report: ## Advisory: duplicate code blocks (reports/dupl.log)
	@mkdir -p reports
	@dupl -threshold 75 $$(go list -f '{{.Dir}}' ./...) > reports/dupl.log 2>&1 || true
	@echo "reports/dupl.log ($$(grep -c 'found' reports/dupl.log 2>/dev/null || true) clone groups)"

goconst-report: ## Advisory: repeated literals (reports/goconst.log)
	@mkdir -p reports
	@goconst -min-occurrences 4 -min-length 5 ./... > reports/goconst.log 2>/dev/null || true
	@echo "reports/goconst.log ($$(wc -l < reports/goconst.log | tr -d ' ') findings)"

audit-test-timings: ## Fail on time.Sleep in tests (TESTS.md §2.1)
	@VIOLATIONS=$$(grep -rn --include='*_test.go' 'time\.Sleep(' . || true); \
	if [ -n "$$VIOLATIONS" ]; then echo "$$VIOLATIONS"; echo "time.Sleep in tests: inject domain/clock instead"; exit 1; fi

vulncheck: ## Check dependencies for known vulnerabilities (pinned govulncheck)
	$(TOOL_ENV) go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

dist: ## Cross-compile release archives and checksums into dist/ (VERSION required)
	@test -n "$(VERSION)" || { echo "VERSION=vX.Y.Z required"; exit 2; }
	@rm -rf $(DIST) && mkdir -p $(DIST)
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; [ "$$os" = windows ] && ext=.exe; \
		name=$(BIN)_$(VERSION)_$${os}_$${arch}; \
		echo "building $$name"; \
		mkdir -p $(DIST)/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w" -o $(DIST)/$$name/$(BIN)$$ext $(CMD); \
		cp LICENSE README.md CHANGELOG.md $(DIST)/$$name/; \
		if [ "$$os" = windows ]; then (cd $(DIST) && zip -qr $$name.zip $$name); \
		else tar -C $(DIST) -czf $(DIST)/$$name.tar.gz $$name; fi; \
		rm -rf $(DIST)/$$name; \
	done
	@cd $(DIST) && sha256sum * > checksums.txt && cat checksums.txt

release: ## Tag and push a release: make release VERSION=vX.Y.Z CONFIRM=1 (see RELEASE.md)
	@test -n "$(VERSION)" || { echo "VERSION=vX.Y.Z required"; exit 2; }
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "VERSION must be vMAJOR.MINOR.PATCH"; exit 2; }
	@test -z "$$(git status --porcelain)" || { echo "working tree is not clean"; exit 1; }
	@test "$$(git rev-parse --abbrev-ref HEAD)" = main || { echo "release from main"; exit 1; }
	@git fetch --quiet origin main --tags
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { echo "main is not in sync with origin/main"; exit 1; }
	@! git rev-parse -q --verify "refs/tags/$(VERSION)" >/dev/null || { echo "tag $(VERSION) already exists; never move a tag"; exit 1; }
	@grep -Eq "^## \[$(patsubst v%,%,$(VERSION))\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$$" CHANGELOG.md || \
		{ echo "CHANGELOG.md has no '## [$(patsubst v%,%,$(VERSION))] - YYYY-MM-DD' heading"; exit 1; }
	@$(MAKE) --no-print-directory check
	@if [ "$(CONFIRM)" != 1 ]; then echo "All gates passed. Re-run with CONFIRM=1 to tag and push $(VERSION)."; exit 0; fi; \
	git tag -a "$(VERSION)" -m "gobuzzkey $(VERSION)" && git push origin "$(VERSION)" && \
	echo "Pushed $(VERSION). The Release workflow builds the binaries and the GitHub release."

tidy: ## go mod tidy + verify
	go mod tidy && go mod verify

clean: ## Remove build outputs, reports and caches
	rm -rf $(BIN).out $(DIST) reports
	go clean -testcache

install: ## Install the pinned development tools
	$(TOOL_ENV) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	$(TOOL_ENV) go install github.com/fe3dback/go-arch-lint@$(GO_ARCH_LINT_VERSION)
	$(TOOL_ENV) go install github.com/mibk/dupl@$(DUPL_VERSION)
	$(TOOL_ENV) go install github.com/jgautheron/goconst/cmd/goconst@$(GOCONST_VERSION)
	$(TOOL_ENV) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

help: ## Show this help
	@echo "gobuzzkey"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
