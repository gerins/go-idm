.DEFAULT_GOAL := help

# Wails installs to GOPATH/bin, which is often not on PATH.
GOBIN  := $(shell go env GOPATH)/bin
export PATH := $(PATH):$(GOBIN)

WAILS ?= wails
BIN   := build/bin
UNAME := $(shell uname -s)

# Copies the extension without dev-only files: $(call copy-extension,<dest>)
define copy-extension
	rm -rf $(1) && mkdir -p $(dir $(1)) && cp -R extension $(1)
	rm -rf $(1)/test $(1)/package.json $(1)/icons/gen.go $(1)/manifest.firefox.json
endef

# Same files, but with the Firefox manifest as manifest.json: $(call copy-firefox-extension,<dest>)
define copy-firefox-extension
	rm -rf $(1) && mkdir -p $(dir $(1)) && cp -R extension $(1)
	rm -rf $(1)/test $(1)/package.json $(1)/icons/gen.go $(1)/manifest.json
	mv $(1)/manifest.firefox.json $(1)/manifest.json
endef

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-26s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Run

.PHONY: dev
dev: build-host build-firefox-extension ## Run the app with live reload (UI also at http://localhost:34115)
	$(WAILS) dev

.PHONY: cli
cli: ## Run the headless CLI, e.g. make cli ARGS="-o ./out https://host/file.zip"
	go run ./cmd/idm-cli $(ARGS)

##@ Build

.PHONY: build
build: ## Production build for the current OS, with the native host and extension bundled
	$(WAILS) build
	$(MAKE) --no-print-directory bundle-extras

.PHONY: bundle-extras
bundle-extras:
ifeq ($(UNAME),Darwin)
	go build -o $(BIN)/GoIDM.app/Contents/MacOS/idm-host ./cmd/idm-host
	$(call copy-extension,$(BIN)/GoIDM.app/Contents/Resources/extension)
	$(call copy-firefox-extension,$(BIN)/GoIDM.app/Contents/Resources/extension-firefox)
else
	go build -o $(BIN)/idm-host ./cmd/idm-host
	$(call copy-extension,$(BIN)/extension)
	$(call copy-firefox-extension,$(BIN)/extension-firefox)
endif

.PHONY: build-windows
build-windows: ## Cross-compile for Windows (goidm.exe, idm-host.exe, extension/ and extension-firefox/ in build/bin)
	$(WAILS) build -platform windows/amd64
	GOOS=windows GOARCH=amd64 go build -o $(BIN)/idm-host.exe ./cmd/idm-host
	$(call copy-extension,$(BIN)/extension)
	$(call copy-firefox-extension,$(BIN)/extension-firefox)

.PHONY: build-host
build-host: ## Build the native messaging host to build/bin/idm-host (used by make dev)
	go build -o $(BIN)/idm-host ./cmd/idm-host

.PHONY: build-windows-installer
build-windows-installer: build-windows ## Windows NSIS installer (needs NSIS installed)
	$(WAILS) build -platform windows/amd64 -nsis

.PHONY: build-firefox-extension
build-firefox-extension: ## Build the Firefox extension folder into build/bin/extension-firefox
	$(call copy-firefox-extension,$(BIN)/extension-firefox)

.PHONY: firefox-zip
firefox-zip: build-firefox-extension ## Zip the Firefox extension (build/bin/goidm-firefox.zip) for signing at addons.mozilla.org
	rm -f $(BIN)/goidm-firefox.zip
	cd $(BIN)/extension-firefox && zip -qr ../goidm-firefox.zip .

.PHONY: build-cli
build-cli: ## Build the headless CLI into build/bin/idm-cli
	go build -o $(BIN)/idm-cli ./cmd/idm-cli

.PHONY: build-all
build-all: build build-windows build-cli ## Build app for this OS and Windows, plus the CLI

##@ Quality

.PHONY: test
test: ## Run Go tests with the race detector
	go test -race -count=1 ./...

.PHONY: ext-test
ext-test: ## Unit-test the browser extension logic (needs Node)
	cd extension && node --test test/*.test.js

.PHONY: check
check: fmt-check vet frontend-check ext-test test ## Everything CI would run

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
