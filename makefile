# Variables
APP_NAME := countdown
BINARY_DIR := $(HOME)/.local/bin
GO := go
GO_BUILD_FLAGS := -ldflags="-s -w"  # Strip the binary to reduce size
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# Default target: Build and install the app
.PHONY: all
all: build install

# Build the binary
.PHONY: build
build:
	@echo "Building $(APP_NAME)..."
	$(GO) mod tidy
	$(GO) build $(GO_BUILD_FLAGS) -o $(APP_NAME) main.go

# Install the binary to the local bin directory (without sudo)
.PHONY: install
install: build
	@echo "Installing $(APP_NAME) to $(BINARY_DIR)..."
	mkdir -p $(BINARY_DIR)
	cp $(APP_NAME) $(BINARY_DIR)/$(APP_NAME)
	@echo "$(APP_NAME) installed to $(BINARY_DIR)"

# Cross-compile a Windows binary (no cgo needed, ebiten's desktop backend
# is pure Go). -H=windowsgui suppresses the console window that would
# otherwise pop up alongside the overlay.
.PHONY: build-windows
build-windows:
	@echo "Building $(APP_NAME).exe for Windows..."
	$(GO) mod tidy
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w -H=windowsgui" -o $(APP_NAME).exe main.go

# Clean up build artifacts
.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -f $(APP_NAME) $(APP_NAME).exe

# Uninstall the binary from the local bin directory
.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(APP_NAME) from $(BINARY_DIR)..."
	rm -f $(BINARY_DIR)/$(APP_NAME)
	@echo "$(APP_NAME) uninstalled."

# Run the app (useful for development)
.PHONY: run
run:
	$(GO) run main.go
