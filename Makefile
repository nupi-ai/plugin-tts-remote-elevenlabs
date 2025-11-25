.PHONY: all build dist test clean release help

# === Adapter identity ===
ADAPTER_NAME ?= $(shell go list -m)
ADAPTER_BINARY ?= tts-remote-elevenlabs
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
VERSION ?= $(shell awk '/^[[:space:]]*version:/{print $$2; exit}' plugin.yaml)
MANIFEST_VERSION := $(shell awk '/^[[:space:]]*version:/{print $$2; exit}' plugin.yaml)
ARTIFACT_BASENAME := $(ADAPTER_BINARY)_$(VERSION)_$(GOOS)_$(GOARCH)
ARTIFACT := dist/$(ARTIFACT_BASENAME).tar.gz
PACKAGE_DIR := dist/.package

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
NC := \033[0m # No Color

all: build

build:
	@echo "$(YELLOW)Building adapter...$(NC)"
	@go build ./...
	@echo "$(GREEN)✓ Build complete$(NC)"

dist: build
	@echo "$(YELLOW)Building distribution binary...$(NC)"
	@mkdir -p dist
	@go build -o dist/$(ADAPTER_BINARY) ./cmd/adapter
	@echo "$(GREEN)✓ Distribution binary built: dist/$(ADAPTER_BINARY)$(NC)"

release: dist
	@echo "$(YELLOW)Preparing release $(VERSION) for $(GOOS)/$(GOARCH)...$(NC)"
	@if [ "$(VERSION)" = "" ]; then \
		echo "$(RED)ERROR: version not found in plugin.yaml$(NC)"; \
		exit 1; \
	fi
	@rm -rf $(PACKAGE_DIR)
	@mkdir -p $(PACKAGE_DIR)
	@cp dist/$(ADAPTER_BINARY) $(PACKAGE_DIR)/
	@cp plugin.yaml $(PACKAGE_DIR)/
	@if [ -f LICENSE ]; then cp LICENSE $(PACKAGE_DIR)/; fi
	@if [ -f README.md ]; then cp README.md $(PACKAGE_DIR)/; fi
	@tar -C $(PACKAGE_DIR) -czf $(ARTIFACT) .
	@python3 -c 'import hashlib, pathlib, sys; p = pathlib.Path(sys.argv[1]); print(f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}")' $(ARTIFACT) > $(ARTIFACT).sha256
	@rm -rf $(PACKAGE_DIR)
	@echo ""
	@echo "$(GREEN)✓ Release artifact created:$(NC)"
	@echo "  $(ARTIFACT)"
	@echo ""
	@echo "$(YELLOW)Checksum:$(NC)"
	@cat $(ARTIFACT).sha256

test:
	@echo "$(YELLOW)Running tests...$(NC)"
	@GOCACHE=$(PWD)/.gocache go test -race ./...
	@echo "$(GREEN)✓ Tests complete$(NC)"

clean:
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	@rm -rf dist
	@go clean ./...
	@echo "$(GREEN)✓ Clean complete$(NC)"

help:
	@echo "$(YELLOW)ElevenLabs TTS Adapter Build System$(NC)"
	@echo ""
	@echo "Available targets:"
	@echo "  $(GREEN)make$(NC)         - Build all packages"
	@echo "  $(GREEN)make build$(NC)   - Build all packages (default)"
	@echo "  $(GREEN)make dist$(NC)    - Build distribution binary"
	@echo "  $(GREEN)make release$(NC) - Create release tarball with checksum"
	@echo "  $(GREEN)make test$(NC)    - Run tests with race detection"
	@echo "  $(GREEN)make clean$(NC)   - Remove all build artifacts"
	@echo "  $(GREEN)make help$(NC)    - Show this help message"
	@echo ""
	@echo "Build outputs:"
	@echo "  Binary:  dist/$(ADAPTER_BINARY)"
	@echo "  Release: $(ARTIFACT)"
