# VCDeploy Makefile
# ==================
# Development-only targets. GoReleaser handles all release artifacts.
# CI uses its own commands - these targets mirror CI for local debugging.

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ============================================================================
# Configuration
# ============================================================================

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

LDFLAGS    := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_TIME)"
OUT_DIR    := bin
PROTO_DIR  := api/proto
PROTO_OUT  := internal/proto

# Detect docker compose command
COMPOSE := $(shell command -v docker-compose 2>/dev/null || echo "docker compose")

# Test configuration (mirrors CI)
TEST_TIMEOUT      := 15m
TEST_ADMIN_PASS   := Admin@Password123!
TEST_HTTP_PORT    := 9000
TEST_HTTP_URL     := http://localhost:$(TEST_HTTP_PORT)
TEST_GRPC_PORT    := 9001
TEST_SSH_PORT     := 2223
TEST_LOG_FILE     := test-server.log
TEST_COMPOSE_FILE := docker/docker-compose.test.yml

# ============================================================================
# Development Build
# ============================================================================

.PHONY: build
build: ## Build binaries for local development
	@mkdir -p $(OUT_DIR)
	go build $(LDFLAGS) -o $(OUT_DIR)/vcdeploy ./cmd/vcdeploy
	go build $(LDFLAGS) -o $(OUT_DIR)/vcdeploy-agent ./cmd/vcdeploy-agent

.PHONY: build-master
build-master: ## Build master binary only
	@mkdir -p $(OUT_DIR)
	go build $(LDFLAGS) -o $(OUT_DIR)/vcdeploy ./cmd/vcdeploy

.PHONY: build-agent
build-agent: ## Build agent binary only
	@mkdir -p $(OUT_DIR)
	go build $(LDFLAGS) -o $(OUT_DIR)/vcdeploy-agent ./cmd/vcdeploy-agent

.PHONY: install-local
install-local: build ## Install binaries to GOPATH/bin
	go install ./cmd/vcdeploy
	go install ./cmd/vcdeploy-agent

.PHONY: clean
clean: ## Remove build artifacts and stop any running servers
	rm -rf $(OUT_DIR) dist/ coverage.out coverage.html data/
	go clean -testcache
	-@pkill -f "vcdeploy master" 2>/dev/null || true
	-@$(COMPOSE) -f $(TEST_COMPOSE_FILE) down -v 2>/dev/null || true

.PHONY: test-cleanup
test-cleanup: ## Clean up test Docker environment (use after interrupted tests)
	@echo "Cleaning up test environment..."
	-@pkill -f "vcdeploy master" 2>/dev/null || true
	-@pkill -f "docker.*logs" 2>/dev/null || true
	-@$(COMPOSE) -f $(TEST_COMPOSE_FILE) down -v 2>/dev/null || true
	-@docker volume rm docker_test-data docker_test-logs docker_agent-test-data docker_agent-test-logs docker_ssh-target-data 2>/dev/null || true
	-@rm -rf data/
	@echo "Test environment cleaned up."

# ============================================================================
# Development Server (for manual testing)
# ============================================================================

.PHONY: dev
dev: build ## Run master in development mode (for manual testing)
	@mkdir -p data
	@[ -L data/templates ] || ln -sf ../web/templates data/templates
	@[ -L data/static ] || ln -sf ../web/static data/static
	VCDEPLOY_DATA_DIR=./data VCDEPLOY_CONFIG_DIR=./configs VCDEPLOY_RUN_DIR=./data VCDEPLOY_LOG_DIR=./data \
		./$(OUT_DIR)/vcdeploy master start --config=configs/master-dev.yaml

.PHONY: dev-agent
dev-agent: build ## Run agent in development mode
	./$(OUT_DIR)/vcdeploy-agent start --config=configs/agent.yaml.example

# ============================================================================
# Testing - Unit Tests (No Server Required)
# ============================================================================

.PHONY: test
test: ## Run unit tests (mirrors CI 'test' job)
	go test -race -short -coverprofile=coverage.out -covermode=atomic ./...

.PHONY: test-coverage
test-coverage: test ## Run tests with coverage report
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: test-systemd
test-systemd: ## Run systemd tests (mirrors CI 'systemd-tests' job)
	go test -v -tags=systemd ./init/...

.PHONY: test-bench
test-bench: ## Run benchmarks (mirrors CI 'benchmarks' job)
	go test -bench=. -benchmem -run=^$$ ./...

# ============================================================================
# Testing - Integration Tests (Starts own SSH container)
# ============================================================================

.PHONY: test-integration
test-integration: ## Run integration tests (mirrors CI 'integration' job)
	@echo "=== Integration Tests (mirrors CI 'integration' job) ==="
	@echo "Cleaning up any existing SSH test container..."
	@docker stop vcdeploy-test-ssh 2>/dev/null || true
	@docker rm vcdeploy-test-ssh 2>/dev/null || true
	@echo "Starting SSH test container..."
	@docker run -d --name vcdeploy-test-ssh \
		-p 2222:2222 \
		-e PUID=1000 -e PGID=1000 -e TZ=UTC \
		-e PASSWORD_ACCESS=true -e USER_NAME=testuser -e USER_PASSWORD=testpass \
		linuxserver/openssh-server >/dev/null 2>&1
	@$(MAKE) -s _wait-for-port PORT=2222 SERVICE="SSH server" || \
		(docker stop vcdeploy-test-ssh 2>/dev/null || true; docker rm vcdeploy-test-ssh 2>/dev/null || true; exit 1)
	@echo "Running integration tests..."
	@TEST_SSH_HOST=localhost TEST_SSH_PORT=2222 TEST_SSH_USER=testuser TEST_SSH_PASS=testpass \
		go test -tags=integration -race -v ./tests/integration/... ; \
	TEST_EXIT=$$?; \
	docker stop vcdeploy-test-ssh 2>/dev/null || true; \
	docker rm vcdeploy-test-ssh 2>/dev/null || true; \
	exit $$TEST_EXIT

# ============================================================================
# Testing - E2E API Tests
# ============================================================================

.PHONY: test-e2e
test-e2e: build ## Run E2E API tests with SSH target (complete tests)
	@echo "=== E2E API Tests (full mode with SSH target) ==="
	@echo "Server logs: $(TEST_LOG_FILE)"
	@$(MAKE) -s _test-env-start SERVICES="master ssh-target"
	@$(MAKE) -s _wait-for-port PORT=$(TEST_SSH_PORT) SERVICE="SSH target" || ($(MAKE) -s test-cleanup; exit 1)
	@API_TOKEN=$$($(MAKE) -s _create-api-token) && \
	echo "Running E2E tests..." && \
	E2E_MASTER_HTTP_URL=$(TEST_HTTP_URL) E2E_API_TOKEN=$$API_TOKEN \
		E2E_ADMIN_USER=admin E2E_ADMIN_PASS=$(TEST_ADMIN_PASS) \
		E2E_TARGET_SSH_HOST=localhost E2E_TARGET_SSH_PORT=$(TEST_SSH_PORT) \
		go test -v -tags=e2e -timeout $(TEST_TIMEOUT) ./tests/e2e/... ; \
	TEST_EXIT=$$?; \
	$(MAKE) -s test-cleanup; \
	exit $$TEST_EXIT

.PHONY: test-e2e-short
test-e2e-short: build ## Run E2E API tests (fast mode, skips SSH-dependent tests)
	@echo "=== E2E API Tests (fast mode) ==="
	@echo "Note: SSH/deploy tests will be skipped. Use 'make test-e2e' for complete tests."
	@echo "Server logs: $(TEST_LOG_FILE)"
	@$(MAKE) -s _test-env-start SERVICES="master"
	@API_TOKEN=$$($(MAKE) -s _create-api-token) && \
	echo "Running E2E tests (fast mode)..." && \
	E2E_MASTER_HTTP_URL=$(TEST_HTTP_URL) E2E_API_TOKEN=$$API_TOKEN \
		E2E_ADMIN_USER=admin E2E_ADMIN_PASS=$(TEST_ADMIN_PASS) \
		E2E_SKIP_SSH_TESTS=1 \
		go test -v -tags=e2e -timeout $(TEST_TIMEOUT) ./tests/e2e/... ; \
	TEST_EXIT=$$?; \
	$(MAKE) -s test-cleanup; \
	exit $$TEST_EXIT

# ============================================================================
# Testing - CLI Tests
# ============================================================================

.PHONY: test-cli
test-cli: build ## Run CLI tests with SSH target (complete tests)
	@echo "=== CLI Tests (full mode with SSH target) ==="
	@echo "Server logs: $(TEST_LOG_FILE)"
	@$(MAKE) -s _test-env-start SERVICES="master ssh-target"
	@$(MAKE) -s _wait-for-port PORT=$(TEST_SSH_PORT) SERVICE="SSH target" || ($(MAKE) -s test-cleanup; exit 1)
	@API_TOKEN=$$($(MAKE) -s _create-api-token) && \
	echo "Running CLI tests..." && \
	VCDEPLOY_BINARY=./$(OUT_DIR)/vcdeploy \
		E2E_MASTER_HTTP_URL=$(TEST_HTTP_URL) E2E_API_TOKEN=$$API_TOKEN \
		E2E_ADMIN_PASS=$(TEST_ADMIN_PASS) \
		E2E_TARGET_SSH_HOST=localhost E2E_TARGET_SSH_PORT=$(TEST_SSH_PORT) \
		go test -v -tags=cli -timeout $(TEST_TIMEOUT) -p=1 ./tests/cli/... ; \
	TEST_EXIT=$$?; \
	$(MAKE) -s test-cleanup; \
	exit $$TEST_EXIT

.PHONY: test-cli-short
test-cli-short: build ## Run CLI tests (fast mode, skips target-dependent tests)
	@echo "=== CLI Tests (fast mode) ==="
	@echo "Note: Target-dependent tests will be skipped. Use 'make test-cli' for complete tests."
	@echo "Server logs: $(TEST_LOG_FILE)"
	@$(MAKE) -s _test-env-start SERVICES="master"
	@API_TOKEN=$$($(MAKE) -s _create-api-token) && \
	echo "Running CLI tests (fast mode)..." && \
	VCDEPLOY_BINARY=./$(OUT_DIR)/vcdeploy \
		E2E_MASTER_HTTP_URL=$(TEST_HTTP_URL) E2E_API_TOKEN=$$API_TOKEN \
		E2E_ADMIN_PASS=$(TEST_ADMIN_PASS) SKIP_AGENT_TESTS=1 \
		go test -v -tags=cli -timeout $(TEST_TIMEOUT) -p=1 ./tests/cli/... ; \
	TEST_EXIT=$$?; \
	$(MAKE) -s test-cleanup; \
	exit $$TEST_EXIT

# ============================================================================
# Testing - UI Tests (Starts own server process)
# ============================================================================

.PHONY: test-ui
test-ui: build ## Run Playwright UI tests (mirrors CI 'ui-tests' job)
	@echo "=== UI Tests (mirrors CI 'ui-tests' job) ==="
	@echo "Cleaning up previous test data..."
	@pkill -f "vcdeploy master" 2>/dev/null || true
	@rm -rf data/
	@cd tests/ui && npm ci && npx playwright install --with-deps chromium
	@$(MAKE) -s _start-local-server
	@cd tests/ui && npx playwright test --workers=1; \
	TEST_EXIT=$$?; \
	$(MAKE) -s _stop-local-server; \
	exit $$TEST_EXIT

# ============================================================================
# Testing - Run All (mirrors CI pipeline)
# ============================================================================

.PHONY: test-all
test-all: test test-systemd test-integration test-e2e test-cli ## Run all tests (mirrors full CI)
	@echo "=== All tests passed! ==="

.PHONY: test-all-short
test-all-short: test test-systemd test-integration test-e2e-short test-cli-short ## Run all tests (fast mode)
	@echo "=== All tests passed (fast mode)! ==="

# ============================================================================
# Testing - Manual Infrastructure (for debugging)
# ============================================================================

.PHONY: test-infra-up
test-infra-up: ## Start full test infrastructure (for debugging)
	@echo "Cleaning up previous Docker test environment..."
	@$(COMPOSE) -f $(TEST_COMPOSE_FILE) down -v 2>/dev/null || true
	@docker volume rm docker_test-data docker_test-logs docker_agent-test-data docker_agent-test-logs 2>/dev/null || true
	TEST_ADMIN_PASSWORD=$(TEST_ADMIN_PASS) $(COMPOSE) -f $(TEST_COMPOSE_FILE) up -d --build --wait
	@echo "Test infrastructure ready!"
	@echo "  Master HTTP: $(TEST_HTTP_URL)"
	@echo "  Master gRPC: localhost:$(TEST_GRPC_PORT)"
	@echo "  SSH Target:  localhost:$(TEST_SSH_PORT)"

.PHONY: test-infra-down
test-infra-down: ## Stop test infrastructure
	$(COMPOSE) -f $(TEST_COMPOSE_FILE) down -v

.PHONY: test-infra-clean
test-infra-clean: test-cleanup ## Clean all test data (alias for test-cleanup)

.PHONY: test-infra-logs
test-infra-logs: ## Show test infrastructure logs
	$(COMPOSE) -f $(TEST_COMPOSE_FILE) logs -f

# ============================================================================
# Code Quality
# ============================================================================

.PHONY: lint
lint: ## Run linter (mirrors CI 'lint' job)
	golangci-lint run --timeout=5m ./...

.PHONY: fmt
fmt: ## Format code
	go fmt ./...
	@command -v gofumpt >/dev/null 2>&1 && gofumpt -l -w . || true

.PHONY: fmt-check
fmt-check: ## Check formatting
	@test -z "$$(gofmt -l . 2>/dev/null | grep -v vendor)" || \
		(echo "Run 'make fmt'" && gofmt -l . | grep -v vendor && exit 1)

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: vuln
vuln: ## Run vulnerability check
	govulncheck ./...

.PHONY: gosec
gosec: ## Run security scanner
	gosec -exclude=G104,G115,G204,G301,G302,G304,G306 -quiet ./...

.PHONY: verify
verify: lint vet vuln test ## Run all verification checks

# ============================================================================
# Code Generation
# ============================================================================

.PHONY: proto
proto: ## Generate protobuf code
	@mkdir -p $(PROTO_OUT)
	protoc --go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/*.proto

.PHONY: proto-check
proto-check: proto ## Verify proto stubs are up to date (mirrors CI)
	@if ! git diff --exit-code internal/proto/; then \
		echo "Proto stubs are out of date. Run 'make proto' and commit."; \
		exit 1; \
	fi
	@echo "Proto stubs are up to date"

.PHONY: generate
generate: ## Run go generate
	go generate ./...

# ============================================================================
# Docker (Development)
# ============================================================================

.PHONY: docker-up
docker-up: ## Start development containers
	$(COMPOSE) -f docker/docker-compose.yml up -d

.PHONY: docker-down
docker-down: ## Stop development containers
	$(COMPOSE) -f docker/docker-compose.yml down

.PHONY: docker-logs
docker-logs: ## Show container logs
	$(COMPOSE) -f docker/docker-compose.yml logs -f

# ============================================================================
# Internal Helpers (not shown in help)
# ============================================================================

# Initialize test environment: cleanup, reset log file, start services
# Usage: $(MAKE) _test-env-start SERVICES="master" or SERVICES="master ssh-target"
_test-env-start:
	@$(MAKE) -s test-cleanup 2>/dev/null || true
	@> $(TEST_LOG_FILE)
	@echo "Building and starting services: $(SERVICES)..."
	@TEST_ADMIN_PASSWORD=$(TEST_ADMIN_PASS) $(COMPOSE) -f $(TEST_COMPOSE_FILE) up -d $(SERVICES) --build
	@$(MAKE) -s _wait-for-master || ($(MAKE) -s _dump-logs-and-cleanup; exit 1)
	@$(COMPOSE) -f $(TEST_COMPOSE_FILE) logs -f $(SERVICES) >> $(TEST_LOG_FILE) 2>&1 &

# Wait for master to be ready (health check)
_wait-for-master:
	@echo "Waiting for master to be ready..."
	@for i in $$(seq 1 30); do \
		if curl -sf $(TEST_HTTP_URL)/api/v1/health >/dev/null 2>&1; then \
			echo "Master is ready!"; \
			exit 0; \
		fi; \
		echo "Attempt $$i/30: Waiting..."; \
		sleep 2; \
	done; \
	echo "Master failed to start within timeout"; \
	exit 1

# Wait for a TCP port to be available
# Usage: $(MAKE) _wait-for-port PORT=2223 SERVICE="SSH target"
_wait-for-port:
	@echo "Waiting for $(SERVICE) on port $(PORT)..."
	@for i in $$(seq 1 30); do \
		if nc -z localhost $(PORT) 2>/dev/null; then \
			echo "$(SERVICE) is ready!"; \
			exit 0; \
		fi; \
		echo "Attempt $$i/30: Waiting for $(SERVICE)..."; \
		sleep 2; \
	done; \
	echo "$(SERVICE) failed to start within timeout"; \
	exit 1

# Dump logs to file and cleanup (used on startup failure)
_dump-logs-and-cleanup:
	@$(COMPOSE) -f $(TEST_COMPOSE_FILE) logs >> $(TEST_LOG_FILE) 2>&1
	@$(MAKE) -s test-cleanup

# Start local test server (non-Docker, for UI tests)
_start-local-server:
	@echo "Starting local test server..."
	@> $(TEST_LOG_FILE)
	@rm -rf data/
	@mkdir -p data
	@[ -L data/templates ] || ln -sf ../web/templates data/templates
	@[ -L data/static ] || ln -sf ../web/static data/static
	@VCDEPLOY_TEST_MODE=true VCDEPLOY_ADMIN_PASSWORD=$(TEST_ADMIN_PASS) \
		VCDEPLOY_DATA_DIR=./data VCDEPLOY_CONFIG_DIR=./configs \
		VCDEPLOY_RUN_DIR=./data VCDEPLOY_LOG_DIR=./data \
		./$(OUT_DIR)/vcdeploy master start --config=configs/master-dev.yaml >> $(TEST_LOG_FILE) 2>&1 &
	@$(MAKE) -s _wait-for-master || (echo "Check $(TEST_LOG_FILE) for details"; cat $(TEST_LOG_FILE); exit 1)

# Stop local test server
_stop-local-server:
	@echo "Stopping local test server..."
	@pkill -f "vcdeploy master" 2>/dev/null || true
	@rm -rf data/

# Create API token and output just the token value
_create-api-token:
	@SESSION=$$(curl -sf -X POST $(TEST_HTTP_URL)/api/v1/auth/login \
		-H "Content-Type: application/json" \
		-d '{"username":"admin","password":"$(TEST_ADMIN_PASS)"}' | jq -r '.token'); \
	if [ -z "$$SESSION" ] || [ "$$SESSION" = "null" ]; then \
		echo "Failed to login" >&2; exit 1; \
	fi; \
	TOKEN=$$(curl -sf -X POST $(TEST_HTTP_URL)/api/v1/api-keys \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $$SESSION" \
		-d '{"name":"test-key-'$$RANDOM'","description":"Test API key","scopes":["*"]}' | jq -r '.key'); \
	if [ -z "$$TOKEN" ] || [ "$$TOKEN" = "null" ]; then \
		echo "Failed to create API token" >&2; exit 1; \
	fi; \
	echo $$TOKEN

# ============================================================================
# Release (GoReleaser)
# ============================================================================

.PHONY: release-snapshot
release-snapshot: ## Build snapshot release (for testing)
	goreleaser release --snapshot --clean

.PHONY: release-check
release-check: ## Validate GoReleaser config (mirrors CI)
	goreleaser check
