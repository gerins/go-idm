.DEFAULT_GOAL := help

# Wails installs to GOPATH/bin, which is often not on PATH.
GOBIN  := $(shell go env GOPATH)/bin
export PATH := $(PATH):$(GOBIN)

WAILS ?= wails
BIN   := build/bin

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-26s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Run

.PHONY: dev
dev: ## Run the app with live reload (UI also at http://localhost:34115)
	$(WAILS) dev

.PHONY: cli
cli: ## Run the headless CLI, e.g. make cli ARGS="-o ./out https://host/file.zip"
	go run ./cmd/idm-cli $(ARGS)

##@ Build

.PHONY: build
build: ## Production build for the current OS
	$(WAILS) build

.PHONY: build-windows
build-windows: ## Cross-compile for Windows (build/bin/goidm.exe)
	$(WAILS) build -platform windows/amd64

.PHONY: build-windows-installer
build-windows-installer: ## Windows NSIS installer (needs NSIS installed)
	$(WAILS) build -platform windows/amd64 -nsis

.PHONY: build-cli
build-cli: ## Build the headless CLI into build/bin/idm-cli
	go build -o $(BIN)/idm-cli ./cmd/idm-cli

.PHONY: build-all
build-all: build build-windows build-cli ## Build app for this OS and Windows, plus the CLI

##@ Quality

.PHONY: test
test: ## Run Go tests with the race detector
	go test -race -count=1 ./...

.PHONY: check
check: fmt-check vet frontend-check test ## Everything CI would run

.PHONY: vet
vet: ## go vet for this OS and Windows
	go vet ./...
	GOOS=windows GOARCH=amd64 go vet ./...

.PHONY: fmt
fmt: ## Format Go code
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any Go file is not gofmt-formatted
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "Unformatted files:"; echo "$$out"; exit 1; fi

.PHONY: frontend-check
frontend-check: ## Type-check the Svelte frontend
	cd frontend && npm run check

##@ Setup

.PHONY: deps
deps: ## Install the Wails CLI and frontend dependencies
	go install github.com/wailsapp/wails/v2/cmd/wails@latest
	cd frontend && npm install

.PHONY: bindings
bindings: ## Regenerate frontend/wailsjs after changing bound Go methods
	$(WAILS) generate module

.PHONY: clean
clean: ## Remove build output and the frontend bundle
	rm -rf $(BIN)
	find frontend/dist -mindepth 1 ! -name gitkeep -delete
