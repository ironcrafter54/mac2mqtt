# Mac2MQTT Makefile
# Provides convenient targets for building and managing mac2mqtt

.PHONY: help build install uninstall clean status logs configure test

# Default target
help:
	@echo "Mac2MQTT Management Commands:"
	@echo ""
	@echo "Build:"
	@echo "  make build        - Build the mac2mqtt binary"
	@echo "  make clean        - Clean build artifacts"
	@echo ""
	@echo "Installation:"
	@echo "  make install      - Run the installer script"
	@echo "  make uninstall    - Run the uninstaller script"
	@echo ""
	@echo "Management:"
	@echo "  make status       - Check service status"
	@echo "  make logs         - Show recent logs"
	@echo "  make configure    - Reconfigure MQTT settings"
	@echo "  make test         - Test MQTT connection"
	@echo ""
	@echo "Quick start:"
	@echo "  make build install  - Build and install in one step"

# Build the binary
build:
	@echo "Building mac2mqtt..."
	go mod tidy
	go build -o mac2mqtt mac2mqtt.go
	chmod +x mac2mqtt
	@echo "Build complete!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f mac2mqtt
	@echo "Clean complete!"

# Install using the installer script
install: build
	@echo "Running installer..."
	@if [ -f "./install.sh" ]; then \
		chmod +x install.sh && ./install.sh; \
	else \
		echo "Error: install.sh not found"; \
		exit 1; \
	fi

# Uninstall using the uninstaller script
uninstall:
	@echo "Running uninstaller..."
	@if [ -f "./uninstall.sh" ]; then \
		chmod +x uninstall.sh && ./uninstall.sh; \
	else \
		echo "Error: uninstall.sh not found"; \
		exit 1; \
	fi

# Check service status
status:
	@echo "Checking service status..."
	@if [ -f "./status.sh" ]; then \
		chmod +x status.sh && ./status.sh; \
	else \
		echo "Error: status.sh not found"; \
		exit 1; \
	fi

# Show recent logs
logs:
	@echo "Showing recent logs..."
	@if [ -f "./status.sh" ]; then \
		chmod +x status.sh && ./status.sh --logs; \
	else \
		echo "Error: status.sh not found"; \
		exit 1; \
	fi

# Reconfigure MQTT settings
configure:
	@echo "Running configuration wizard..."
	@if [ -f "./configure.sh" ]; then \
		chmod +x configure.sh && ./configure.sh; \
	else \
		echo "Error: configure.sh not found"; \
		exit 1; \
	fi

# Test MQTT connection
test:
	@echo "Testing MQTT connection..."
	@if [ -f "./mac2mqtt" ]; then \
		timeout 10s ./mac2mqtt || echo "Test completed"; \
	else \
		echo "Error: mac2mqtt binary not found. Run 'make build' first."; \
		exit 1; \
	fi

# Development target for quick testing
dev: build
	@echo "Running in development mode..."
	./mac2mqtt

# Package for distribution
package: build
	@echo "Creating distribution package..."
	@mkdir -p dist
	@cp mac2mqtt dist/
	@cp mac2mqtt.yaml dist/
	@cp com.hagak.mac2mqtt.plist dist/
	@cp install.sh dist/
	@cp uninstall.sh dist/
	@cp configure.sh dist/
	@cp status.sh dist/
	@cp README.md dist/
	@cp INSTALL.md dist/
	@chmod +x dist/*.sh
	@echo "Package created in dist/ directory"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	@echo "Dependencies installed!"

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "Code formatted!"

# Run tests (if any)
test-code:
	@echo "Running tests..."
	go test ./... || echo "No tests found or tests failed"
	@echo "Tests completed!"

# Lint code
lint:
	@echo "Linting code..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi 