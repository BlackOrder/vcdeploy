# VCDeploy Makefile
# ==================

.PHONY: all build build-master build-agent clean test test-unit test-integration \
        test-systemd test-e2e test-e2e-api test-cli test-ui test-all test-all-no-parallel \
        test-coverage test-coverage-ci test-bench \
        lint fmt vet proto vuln gosec sbom security quality-check \
        install install-systemd uninstall dev dev-agent \
        docker-build docker-up docker-down docker-test-up docker-test-down \
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

## test-e2e: Run full end-to-end tests (requires Docker)
test-e2e:
	@echo "Running e2e tests..."
	@if [ ! -f docker-compose.test.yaml ]; then \
		echo "Error: docker-compose.test.yaml not found"; \
		exit 1; \
	fi
	@echo "Starting test infrastructure..."
	$(DOCKER_COMPOSE) -f docker-compose.test.yaml up -d --build --wait || \
		(echo "Failed to start containers"; exit 1)
	@echo "Running e2e API tests..."
	go test -v -tags=e2e -timeout $(TEST_TIMEOUT) ./tests/e2e/... || \
		($(DOCKER_COMPOSE) -f docker-compose.test.yaml down -v; exit 1)
	@echo "Cleaning up..."
	$(DOCKER_COMPOSE) -f docker-compose.test.yaml down -v

## test-e2e-api: Run E2E API tests only (requires running infrastructure)
test-e2e-api:
	@echo "Running E2E API tests..."
	go test -v -tags=e2e -timeout $(TEST_TIMEOUT) ./tests/e2e/...

## test-cli: Run CLI tests (requires running infrastructure)
test-cli:
	@echo "Running CLI tests..."
	go test -v -tags=cli -timeout $(TEST_TIMEOUT) ./tests/cli/...

## test-ui: Run Playwright UI tests (requires running infrastructure)
test-ui:
	@echo "Running Playwright UI tests..."
	@cd tests/ui && npm install && npx playwright test

## test-ui-headed: Run Playwright UI tests with visible browser
test-ui-headed:
	@echo "Running Playwright UI tests (headed)..."
	@cd tests/ui && npm install && npx playwright test --headed

## test-ui-debug: Run Playwright UI tests in debug mode
test-ui-debug:
	@echo "Running Playwright UI tests (debug)..."
	@cd tests/ui && npm install && npx playwright test --debug

## test-ui-report: Show Playwright test report
test-ui-report:
	@echo "Opening Playwright report..."
	@cd tests/ui && npx playwright show-report

## test-all: Run all tests (unit + integration + systemd, excludes e2e)
test-all:
	@echo "Running all tests..."
	go test $(TEST_FLAGS) -tags=integration,systemd -timeout $(TEST_TIMEOUT) ./...

## test-all-no-parallel: Run all tests in single-worker mode
test-all-no-parallel:
	@echo "Running all tests (single-worker mode)..."
	TEST_NO_PARALLEL=1 go test $(TEST_FLAGS) -p 1 -tags=integration,systemd -timeout $(TEST_TIMEOUT) ./...

## test-full: Run all tests including E2E, CLI, and UI (requires Docker)
test-full:
	@echo "Running full test suite..."
	@$(MAKE) test-all
	@$(MAKE) test-e2e
	@$(MAKE) test-cli
	@$(MAKE) test-ui

## test-full-no-parallel: Run full test suite in single-worker mode
test-full-no-parallel:
	@echo "Running full test suite (single-worker mode)..."
	TEST_NO_PARALLEL=1 $(MAKE) test-all-no-parallel
	TEST_NO_PARALLEL=1 go test -v -p 1 -tags=e2e -timeout $(TEST_TIMEOUT) ./tests/e2e/...
	TEST_NO_PARALLEL=1 go test -v -p 1 -tags=cli -timeout $(TEST_TIMEOUT) ./tests/cli/...
	@cd tests/ui && TEST_NO_PARALLEL=1 npx playwright test --workers=1

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

## quality-check: Run all quality checks using script
quality-check:
	@echo "Running quality checks..."
	@./scripts/quality-check.sh

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

## fmt-check: Check if code is formatted (for CI)
fmt-check:
	@echo "Checking code formatting..."
	@test -z "$$(gofmt -l . 2>/dev/null | grep -v vendor)" || \
		(echo "Code is not properly formatted. Run 'make fmt' to fix."; gofmt -l . | grep -v vendor; exit 1)

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
	@mkdir -p data
	@[ -L data/templates ] || ln -sf ../web/templates data/templates
	@[ -L data/static ] || ln -sf ../web/static data/static
	VCDEPLOY_DATA_DIR=./data VCDEPLOY_CONFIG_DIR=./configs VCDEPLOY_RUN_DIR=./data VCDEPLOY_LOG_DIR=./data go run ./cmd/vcdeploy master start --config=configs/master-dev.yaml

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

## docker-test-up: Start test infrastructure (master, agent, gitea, ssh-target)
docker-test-up:
	@echo "Starting test infrastructure..."
	$(DOCKER_COMPOSE) -f docker/docker-compose.test.yml up -d --build --wait
	@echo "Test infrastructure ready!"
	@echo "  Master HTTP: http://localhost:8080"
	@echo "  Master gRPC: localhost:9090"
	@echo "  Git Server:  http://localhost:3000"
	@echo "  SSH Target:  localhost:2223"

## docker-test-down: Stop test infrastructure
docker-test-down:
	@echo "Stopping test infrastructure..."
	$(DOCKER_COMPOSE) -f docker/docker-compose.test.yml down -v

## docker-test-logs: Show test infrastructure logs
docker-test-logs:
	$(DOCKER_COMPOSE) -f docker/docker-compose.test.yml logs -f

## docker-test-status: Show test infrastructure status
docker-test-status:
	$(DOCKER_COMPOSE) -f docker/docker-compose.test.yml ps

## docker-test-playwright: Run Playwright UI tests in Docker
docker-test-playwright:
	@echo "Running Playwright tests in Docker..."
	$(DOCKER_COMPOSE) -f docker/docker-compose.test.yml --profile ui-tests up playwright --abort-on-container-exit
	$(DOCKER_COMPOSE) -f docker/docker-compose.test.yml --profile ui-tests rm -f playwright

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
	@grep -E '^## (lint|fmt|vet|check|verify|quality):' Makefile | sed 's/## /  /'
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
