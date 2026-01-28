# VCDeploy Makefile
# ==================

.PHONY: all build build-master build-agent clean test test-unit test-integration \
        test-systemd test-e2e test-all test-coverage test-coverage-ci test-bench \
        lint fmt vet proto vuln gosec sbom security \
        install install-systemd uninstall dev dev-agent \
        docker-build docker-up docker-down \
        check verify help

# ------------------------------------------------------------------------------
# Configuration
# ------------------------------------------------------------------------------

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

LDFLAGS    := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"
OUT_DIR    := bin
PROTO_DIR  := api/proto
PROTO_OUT  := internal/proto

# Detect docker compose command (v2 uses 'docker compose', v1 uses 'docker-compose')
DOCKER_COMPOSE := $(shell command -v docker-compose 2>/dev/null || echo "docker compose")

# Test flags
TEST_FLAGS     := -v -race
TEST_TIMEOUT   := 10m
COVERAGE_FILE  := coverage.out

# ------------------------------------------------------------------------------
# Default target
# ------------------------------------------------------------------------------

all: build

# ------------------------------------------------------------------------------
# Build targets
# ------------------------------------------------------------------------------

## build: Build both master and agent binaries
build: build-master build-agent

## build-master: Build the master binary
build-master:
	@echo "Building vcdeploy master..."
	@mkdir -p $(OUT_DIR)
	go build $(LDFLAGS) -o $(OUT_DIR)/vcdeploy ./cmd/vcdeploy

## build-agent: Build the agent binary
build-agent:
	@echo "Building vcdeploy agent..."
	@mkdir -p $(OUT_DIR)
	go build $(LDFLAGS) -o $(OUT_DIR)/vcdeploy-agent ./cmd/vcdeploy-agent

## clean: Remove build artifacts and test cache
clean:
	@echo "Cleaning..."
	@rm -rf $(OUT_DIR)
	@rm -rf $(PROTO_OUT)/*.pb.go
	@rm -f $(COVERAGE_FILE) coverage.html
	go clean -testcache

# ------------------------------------------------------------------------------
# Test targets
# ------------------------------------------------------------------------------

## test: Run unit tests only (fast, no external dependencies)
test: test-unit

## test-unit: Run unit tests (excludes integration, e2e, systemd)
test-unit:
	@echo "Running unit tests..."
	go test $(TEST_FLAGS) -short -timeout $(TEST_TIMEOUT) ./...

## test-integration: Run integration tests (requires Docker)
test-integration:
	@echo "Running integration tests..."
	go test $(TEST_FLAGS) -tags=integration -timeout $(TEST_TIMEOUT) ./...

## test-systemd: Run systemd unit validation tests
test-systemd:
	@echo "Running systemd tests..."
	go test -v -tags=systemd ./init/...

## test-e2e: Run end-to-end tests (requires Docker)
test-e2e:
	@echo "Running e2e tests..."
	@if [ ! -f docker-compose.test.yaml ]; then \
		echo "Error: docker-compose.test.yaml not found"; \
		exit 1; \
	fi
	@echo "Starting test infrastructure..."
	$(DOCKER_COMPOSE) -f docker-compose.test.yaml up -d --build --wait || \
		(echo "Failed to start containers"; exit 1)
	@echo "Running e2e tests..."
	go test -v -tags=e2e -timeout $(TEST_TIMEOUT) ./tests/e2e/... || \
		($(DOCKER_COMPOSE) -f docker-compose.test.yaml down -v; exit 1)
	@echo "Cleaning up..."
	$(DOCKER_COMPOSE) -f docker-compose.test.yaml down -v

## test-all: Run all tests (unit + integration + systemd, excludes e2e)
test-all:
	@echo "Running all tests..."
	go test $(TEST_FLAGS) -tags=integration,systemd -timeout $(TEST_TIMEOUT) ./...

## test-coverage: Run tests with HTML coverage report
test-coverage:
	@echo "Running tests with coverage..."
	go test $(TEST_FLAGS) -coverprofile=$(COVERAGE_FILE) -timeout $(TEST_TIMEOUT) ./...
	go tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "Coverage report: coverage.html"

## test-coverage-ci: Run tests with coverage (CI mode, text output)
test-coverage-ci:
	@echo "Running tests with coverage (CI mode)..."
	go test $(TEST_FLAGS) -coverprofile=$(COVERAGE_FILE) -covermode=atomic -timeout $(TEST_TIMEOUT) ./...
	go tool cover -func=$(COVERAGE_FILE)

## test-bench: Run benchmarks
test-bench:
	@echo "Running benchmarks..."
	go test -v -bench=. -benchmem -run=^$$ ./...

# ------------------------------------------------------------------------------
# Security & SBOM
# ------------------------------------------------------------------------------

## vuln: Run vulnerability check
vuln:
	@echo "Running vulnerability check..."
	@command -v govulncheck >/dev/null 2>&1 || \
		(echo "govulncheck not installed. Run: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1)
	govulncheck ./...

## gosec: Run security scanner
gosec:
	@echo "Running security scanner..."
	@command -v gosec >/dev/null 2>&1 || \
		(echo "gosec not installed. Run: go install github.com/securego/gosec/v2/cmd/gosec@latest"; exit 1)
	gosec -exclude=G104,G115,G204,G301,G302,G304,G306 ./...

## sbom: Generate Software Bill of Materials (SBOM)
sbom:
	@echo "Generating SBOM..."
	@command -v cyclonedx-gomod >/dev/null 2>&1 || \
		(echo "cyclonedx-gomod not installed. Run: go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest"; exit 1)
	@mkdir -p $(OUT_DIR)
	cyclonedx-gomod mod -json -output $(OUT_DIR)/sbom.json
	cyclonedx-gomod mod -output $(OUT_DIR)/sbom.xml
	@echo "SBOM generated: $(OUT_DIR)/sbom.json and $(OUT_DIR)/sbom.xml"

## security: Run all security checks (vuln + gosec)
security: vuln gosec

## check: Run all quality checks (lint + vet + test) - good for CI
check: lint vet test

## verify: Run full verification (lint + security + test) - thorough check
verify: lint security test

# ------------------------------------------------------------------------------
# Code quality
# ------------------------------------------------------------------------------

## lint: Run linter
lint:
	@echo "Running linter..."
	@command -v golangci-lint >/dev/null 2>&1 || \
		(echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1)
	golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# ------------------------------------------------------------------------------
# Code generation
# ------------------------------------------------------------------------------

## proto: Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@mkdir -p $(PROTO_OUT)
	protoc --go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/*.proto

# ------------------------------------------------------------------------------
# Installation
# ------------------------------------------------------------------------------

## install: Install binaries to /usr/local/bin
install: build
	@echo "Installing binaries..."
	sudo cp $(OUT_DIR)/vcdeploy /usr/local/bin/
	sudo cp $(OUT_DIR)/vcdeploy-agent /usr/local/bin/
	@echo "Installed to /usr/local/bin/"

## install-systemd: Install systemd service files
install-systemd:
	@echo "Installing systemd services..."
	sudo cp init/vcdeploy-master.service /etc/systemd/system/
	sudo cp init/vcdeploy-agent.service /etc/systemd/system/
	sudo systemctl daemon-reload
	@echo "Systemd services installed. Enable with:"
	@echo "  sudo systemctl enable --now vcdeploy-master"
	@echo "  sudo systemctl enable --now vcdeploy-agent"

## uninstall: Remove installed binaries
uninstall:
	@echo "Uninstalling binaries..."
	sudo rm -f /usr/local/bin/vcdeploy /usr/local/bin/vcdeploy-agent
	@echo "Uninstalled from /usr/local/bin/"

# ------------------------------------------------------------------------------
# Development
# ------------------------------------------------------------------------------

## dev: Run master in development mode
dev:
	go run ./cmd/vcdeploy master start --config=configs/master.yaml.example

## dev-agent: Run agent in development mode
dev-agent:
	go run ./cmd/vcdeploy-agent start --config=configs/agent.yaml.example

# ------------------------------------------------------------------------------
# Docker
# ------------------------------------------------------------------------------

## docker-build: Build Docker images
docker-build:
	@echo "Building Docker images..."
	docker build -t vcdeploy:$(VERSION) -f docker/Dockerfile .
	docker build -t vcdeploy-agent:$(VERSION) -f docker/Dockerfile.agent .

## docker-up: Start development containers
docker-up:
	$(DOCKER_COMPOSE) -f docker/docker-compose.yml up -d

## docker-down: Stop development containers
docker-down:
	$(DOCKER_COMPOSE) -f docker/docker-compose.yml down

# ------------------------------------------------------------------------------
# Help
# ------------------------------------------------------------------------------

## help: Show this help
help:
	@echo "vcdeploy - Deployment Platform"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build:"
	@grep -E '^## (build|clean):' Makefile | sed 's/## /  /'
	@echo ""
	@echo "Test:"
	@grep -E '^## test' Makefile | sed 's/## /  /'
	@echo ""
	@echo "Code Quality:"
	@grep -E '^## (lint|fmt|vet|check|verify):' Makefile | sed 's/## /  /'
	@echo ""
	@echo "Security:"
	@grep -E '^## (vuln|gosec|sbom|security):' Makefile | sed 's/## /  /'
	@echo ""
	@echo "Installation:"
	@grep -E '^## (install|uninstall)' Makefile | sed 's/## /  /'
	@echo ""
	@echo "Development:"
	@grep -E '^## (dev|docker)' Makefile | sed 's/## /  /'
	@echo ""
	@echo "Other:"
	@grep -E '^## (proto|help):' Makefile | sed 's/## /  /'
