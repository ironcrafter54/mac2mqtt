# Mac2MQTT Makefile

# Variables
BINARY_NAME=mac2mqtt
VERSION=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -s -w"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
CGO_ENABLED=1

.PHONY: all build clean test deps help build-all build-amd64 build-arm64 install uninstall status

all: clean deps test build

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build for current architecture
	@echo "Building $(BINARY_NAME) for current architecture..."
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) mac2mqtt.go
	@echo "Build complete: $(BINARY_NAME)"

build-all: build-amd64 build-arm64 ## Build for both Intel and ARM architectures

build-amd64: ## Build for Intel Mac (amd64)
	@echo "Building $(BINARY_NAME) for Intel Mac (amd64)..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 mac2mqtt.go
	chmod +x $(BINARY_NAME)-darwin-amd64
	@echo "Build complete: $(BINARY_NAME)-darwin-amd64"

build-arm64: ## Build for Apple Silicon Mac (arm64)
	@echo "Building $(BINARY_NAME) for Apple Silicon Mac (arm64)..."
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 mac2mqtt.go
	chmod +x $(BINARY_NAME)-darwin-arm64
	@echo "Build complete: $(BINARY_NAME)-darwin-arm64"

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME) $(BINARY_NAME)-darwin-*
	@echo "Clean complete"

test: ## Run tests
	@echo "Running tests..."
	$(GOTEST) -v ./...
	@echo "Tests complete"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	@echo "Dependencies downloaded"

install: ## Install Mac2MQTT
	@echo "Installing Mac2MQTT..."
	./install.sh

uninstall: ## Uninstall Mac2MQTT
	@echo "Uninstalling Mac2MQTT..."
	./uninstall.sh

status: ## Check Mac2MQTT status
	@echo "Checking Mac2MQTT status..."
	./status.sh

debug: ## Run debug script
	@echo "Running debug script..."
	./debug.sh

run: build ## Build and run locally
	@echo "Running Mac2MQTT locally..."
	./$(BINARY_NAME)

release: build-all ## Build release packages
	@echo "Creating release packages..."
	@for arch in amd64 arm64; do \
		target="darwin-$$arch"; \
		echo "Creating package for $$target..."; \
		tar -czf $(BINARY_NAME)-$$target.tar.gz \
			$(BINARY_NAME)-$$target \
			mac2mqtt.yaml \
			com.hagak.mac2mqtt.plist \
			install.sh \
			status.sh \
			debug.sh \
			README.md \
			INSTALL.md; \
	done
	@echo "Release packages created:"
	@ls -la $(BINARY_NAME)-darwin-*.tar.gz

format: ## Format Go code
	@echo "Formatting Go code..."
	$(GOCMD) fmt ./...
	@echo "Formatting complete"

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOCMD) vet ./...
	@echo "Vet complete"

lint: format vet ## Run linting tools

# Development helpers
dev-setup: ## Set up development environment
	@echo "Setting up development environment..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Development setup complete"

dev-test: ## Run tests with coverage
	@echo "Running tests with coverage..."
	$(GOTEST) -v -cover ./...
	@echo "Test coverage complete"

# GitHub Actions helpers
gh-build: ## Build for GitHub Actions
	@echo "Building for GitHub Actions..."
	$(GOBUILD) -ldflags="-s -w" -o $(BINARY_NAME) mac2mqtt.go
	chmod +x $(BINARY_NAME)
	@echo "GitHub Actions build complete"

gh-build-matrix: ## Build for GitHub Actions matrix
	@echo "Building for architecture: $(GOARCH)"
	$(GOBUILD) -ldflags="-s -w" -o $(BINARY_NAME)-darwin-$(GOARCH) mac2mqtt.go
	chmod +x $(BINARY_NAME)-darwin-$(GOARCH)
	@echo "Matrix build complete: $(BINARY_NAME)-darwin-$(GOARCH)" 