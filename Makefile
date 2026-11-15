# ctxmap — run `make help` for the list of targets.
#
# Requires GNU make and a POSIX shell (on Windows: run from Git Bash, or have
# Git's bash on PATH). Dev tools are pinned below and installed into
# ./bin/tools on first use, so nothing global is required besides Go.

SHELL         := bash
.SHELLFLAGS   := -eu -o pipefail -c
.DEFAULT_GOAL := help
.DELETE_ON_ERROR:
MAKEFLAGS     += --no-builtin-rules --no-print-directory

# ---- project ---------------------------------------------------------------
BINARY  := ctxmap
GO      ?= go
GOEXE   := $(shell $(GO) env GOEXE)
BIN_DIR := bin
BIN     := $(BIN_DIR)/$(BINARY)$(GOEXE)
PKGS    := ./...

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
# -trimpath: reproducible builds without local paths; CGO off: static binary
BUILD_FLAGS := -trimpath -ldflags "$(LDFLAGS)"
export CGO_ENABLED ?= 0

COVER_PROFILE := coverage.out

# Where `make install-skill` puts the Claude Code skill (user-level skills).
CLAUDE_SKILLS_DIR ?= $(HOME)/.claude/skills

# ---- pinned tools ------------------------------------------------------------
TOOLS_DIR             := $(BIN_DIR)/tools
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION   := v1.9.0
# Versioned file names: bumping a version above reinstalls automatically.
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)$(GOEXE)
GOVULNCHECK   := $(TOOLS_DIR)/govulncheck-$(GOVULNCHECK_VERSION)$(GOEXE)

##@ General

.PHONY: help
help: ## Show this help
	@awk 'BEGIN { FS = ":.*## "; printf "Usage: make \033[36m<target>\033[0m [VAR=value]\n" } \
		/^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@printf "\nVariables: VERSION=%s GO=%s\n" "$(VERSION)" "$(GO)"

##@ Development

.PHONY: fmt
fmt: $(GOLANGCI_LINT) ## Format code (gofmt + goimports)
	$(GOLANGCI_LINT) fmt

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKGS)

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint (config: .golangci.yml)
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Run golangci-lint and apply automatic fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

.PHONY: run
run: ## Run ctxmap on this repo, e.g. make run ARGS="slice Map"
	$(GO) run . $(or $(ARGS),map)

##@ Testing

.PHONY: test
test: ## Run unit tests
	$(GO) test $(PKGS)

.PHONY: test-race
test-race: export CGO_ENABLED = 1
test-race: ## Run unit tests with the race detector (needs a C compiler)
	$(GO) test -race $(PKGS)

.PHONY: cover
cover: ## Run tests with coverage and print the total
	$(GO) test -covermode=atomic -coverprofile=$(COVER_PROFILE) $(PKGS)
	$(GO) tool cover -func=$(COVER_PROFILE) | tail -n 1

.PHONY: cover-html
cover-html: cover ## Open the coverage report in a browser
	$(GO) tool cover -html=$(COVER_PROFILE)

.PHONY: smoke
smoke: build ## Run the built binary against this repository
	$(BIN) map > /dev/null
	$(BIN) deps > /dev/null
	$(BIN) deps --target parser > /dev/null
	$(BIN) slice Map > /dev/null
	@echo "smoke: ok"

##@ Quality gates

.PHONY: fmt-check
fmt-check: $(GOLANGCI_LINT) ## Fail if any file needs formatting
	$(GOLANGCI_LINT) fmt --diff

.PHONY: tidy-check
tidy-check: ## Fail if go.mod / go.sum are not tidy
	$(GO) mod tidy -diff

.PHONY: vuln
vuln: $(GOVULNCHECK) ## Scan for known vulnerabilities (govulncheck)
	$(GOVULNCHECK) $(PKGS)

.PHONY: check
check: fmt-check vet lint test ## Fast pre-commit checks

.PHONY: ci
ci: tidy-check fmt-check vet lint test smoke ## Everything CI should run

##@ Build

.PHONY: build
build: ## Build ./bin/ctxmap
	$(GO) build $(BUILD_FLAGS) -o $(BIN) .

.PHONY: install
install: ## Install ctxmap into $GOBIN (or $GOPATH/bin)
	$(GO) install $(BUILD_FLAGS) .

.PHONY: install-skill
install-skill: ## Install the Claude Code skill into ~/.claude/skills/ctxmap
	@mkdir -p "$(CLAUDE_SKILLS_DIR)/ctxmap"
	cp skills/ctxmap/SKILL.md "$(CLAUDE_SKILLS_DIR)/ctxmap/SKILL.md"
	@echo "installed: $(CLAUDE_SKILLS_DIR)/ctxmap/SKILL.md (start a new Claude Code session to pick it up)"

.PHONY: clean
clean: ## Remove build and coverage artifacts
	rm -f $(BIN) $(COVER_PROFILE)

.PHONY: clean-all
clean-all: clean ## Also remove installed dev tools (./bin)
	rm -rf $(BIN_DIR)

.PHONY: tools
tools: $(GOLANGCI_LINT) $(GOVULNCHECK) ## Install pinned dev tools into ./bin/tools

# ---- tool installation -------------------------------------------------------
# go-install-tool <target-file> <package> <version> <binary-name>
define go-install-tool
	@mkdir -p $(TOOLS_DIR)
	@echo "installing $(2)@$(3)"
	GOBIN="$(abspath $(TOOLS_DIR))" $(GO) install $(2)@$(3)
	@mv "$(TOOLS_DIR)/$(4)$(GOEXE)" "$(1)"
endef

$(GOLANGCI_LINT):
	$(call go-install-tool,$@,github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION),golangci-lint)

$(GOVULNCHECK):
	$(call go-install-tool,$@,golang.org/x/vuln/cmd/govulncheck,$(GOVULNCHECK_VERSION),govulncheck)
