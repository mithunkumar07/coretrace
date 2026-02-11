# CoreTrace Agent Makefile

# Variables
APP_NAME = coretrace-agent
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_SHA = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build settings
BUILD_DIR = bin
LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.commitSHA=$(COMMIT_SHA)"

# Docker settings
DOCKER_REGISTRY ?= coretrace
DOCKER_TAG ?= $(VERSION)
DOCKER_IMAGE = $(DOCKER_REGISTRY)/$(APP_NAME):$(DOCKER_TAG)

.PHONY: help build clean test docker-build docker-push lint format deps

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: ## Build the agent binary
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) .

build-debug: ## Build with debug symbols
	@echo "Building $(APP_NAME) in debug mode..."
	@mkdir -p $(BUILD_DIR)
	go build -gcflags="all=-N -l" $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) .

build-release: ## Build optimized release binary
	@echo "Building $(APP_NAME) for release..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -a -installsuffix cgo $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) .

build-all: ## Build for multiple architectures
	@echo "Building $(APP_NAME) for multiple architectures..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 .

# Development targets
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

format: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...
	goimports -w .

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Docker targets
docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_IMAGE)..."
	docker build -t $(DOCKER_IMAGE) --build-arg VERSION=$(VERSION) --build-arg BUILD_TIME=$(BUILD_TIME) .

docker-build-release: ## Build Docker image with optimizations
	@echo "Building optimized Docker image $(DOCKER_IMAGE)..."
	docker build -t $(DOCKER_IMAGE) --target release --build-arg VERSION=$(VERSION) --build-arg BUILD_TIME=$(BUILD_TIME) .

docker-push: docker-build ## Push Docker image to registry
	@echo "Pushing Docker image $(DOCKER_IMAGE)..."
	docker push $(DOCKER_IMAGE)

docker-run: docker-build ## Run Docker container
	@echo "Running Docker container..."
	docker run --rm --privileged -v /var/log:/var/log:ro -v $(shell pwd)/config.yaml:/etc/coretrace/config.yaml:ro $(DOCKER_IMAGE) /usr/local/bin/coretrace-agent --debug

# Installation targets
install: build ## Install binary to /usr/local/bin
	@echo "Installing $(APP_NAME) to /usr/local/bin..."
	sudo cp $(BUILD_DIR)/$(APP_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(APP_NAME)

uninstall: ## Remove binary from /usr/local/bin
	@echo "Removing $(APP_NAME) from /usr/local/bin..."
	sudo rm -f /usr/local/bin/$(APP_NAME)

# Development helpers
dev-run: build ## Run the agent in development mode
	@echo "Running $(APP_NAME) in development mode..."
	sudo ./$(BUILD_DIR)/$(APP_NAME) monitor --debug

dev-config: ## Copy config to /etc/coretrace
	@echo "Creating config directory and copying config..."
	sudo mkdir -p /etc/coretrace
	sudo cp config.yaml /etc/coretrace/config.yaml

# Release helpers
tag: ## Create and push git tag
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make tag VERSION=v1.0.0"; \
		exit 1; \
	fi
	@echo "Creating git tag $(VERSION)..."
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)

# Information targets
info: ## Show build information
	@echo "App: $(APP_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Commit SHA: $(COMMIT_SHA)"
	@echo "Docker Image: $(DOCKER_IMAGE)"