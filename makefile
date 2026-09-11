# Variables
APP_NAME := countdown
BUILD_DIR := bin
INSTALL_DIR := $(HOME)/.local/bin
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
	mkdir -p $(BUILD_DIR)
	$(GO) build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(APP_NAME) main.go

# Install the binary to the local bin directory (without sudo)
.PHONY: install
install: build
	@echo "Installing $(APP_NAME) to $(INSTALL_DIR)..."
	mkdir -p $(INSTALL_DIR)
	cp $(BUILD_DIR)/$(APP_NAME) $(INSTALL_DIR)/$(APP_NAME)
	@echo "$(APP_NAME) installed to $(INSTALL_DIR)"

# Cross-compile a Windows binary (no cgo needed, ebiten's desktop backend
# is pure Go). -H=windowsgui suppresses the console window that would
# otherwise pop up alongside the overlay.
.PHONY: build-windows
build-windows:
	@echo "Building $(APP_NAME).exe for Windows..."
	$(GO) mod tidy
	mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w -H=windowsgui" -o $(BUILD_DIR)/$(APP_NAME).exe main.go

# Clean up build artifacts
.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)

# Uninstall the binary from the local bin directory
.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(APP_NAME) from $(INSTALL_DIR)..."
	rm -f $(INSTALL_DIR)/$(APP_NAME)
	@echo "$(APP_NAME) uninstalled."

# Run the app (useful for development)
.PHONY: run
run:
	$(GO) run main.go
