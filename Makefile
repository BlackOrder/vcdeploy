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
TEST_TIMEOUT     := 15m
TEST_ADMIN_PASS  := Admin@Password123!
TEST_HTTP_PORT   := 9000
TEST_HTTP_URL    := http://localhost:$(TEST_HTTP_PORT)
TEST_GRPC_PORT   := 9001

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
	-@$(COMPOSE) -f docker/docker-compose.test.yml down -v 2>/dev/null || true

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
	@echo "Starting SSH test container..."
	@docker run -d --name vcdeploy-test-ssh --rm \
		-p 2222:2222 \
		-e PUID=1000 -e PGID=1000 -e TZ=UTC \
		-e PASSWORD_ACCESS=true -e USER_NAME=testuser -e USER_PASSWORD=testpass \
		linuxserver/openssh-server >/dev/null 2>&1 || true
	@echo "Waiting for SSH server..."
	@for i in $$(seq 1 30); do nc -z localhost 2222 2>/dev/null && break || sleep 1; done
	@echo "Running integration tests..."
	@TEST_SSH_HOST=localhost TEST_SSH_PORT=2222 TEST_SSH_USER=testuser TEST_SSH_PASS=testpass \
		go test -tags=integration -race -v ./tests/integration/... ; \
	TEST_EXIT=$$?; \
	docker stop vcdeploy-test-ssh 2>/dev/null || true; \
	exit $$TEST_EXIT

# ============================================================================
# Testing - E2E API Tests (Starts own server process)
# ============================================================================

.PHONY: test-e2e
test-e2e: build ## Run E2E API tests (mirrors CI 'e2e-api' job)
	@echo "=== E2E API Tests (mirrors CI 'e2e-api' job) ==="
	@$(MAKE) -s _start-test-server
	@API_TOKEN=$$($(MAKE) -s _create-api-token) && \
	echo "Running E2E tests..." && \
	E2E_MASTER_HTTP_URL=$(TEST_HTTP_URL) E2E_API_TOKEN=$$API_TOKEN \
		E2E_ADMIN_USER=admin E2E_ADMIN_PASS=$(TEST_ADMIN_PASS) \
		go test -v -tags=e2e -timeout $(TEST_TIMEOUT) ./tests/e2e/... ; \
	TEST_EXIT=$$?; \
	$(MAKE) -s _stop-test-server; \
	exit $$TEST_EXIT

# ============================================================================
# Testing - CLI Tests (Starts own server via Docker)
# ============================================================================

.PHONY: test-cli
test-cli: build ## Run CLI tests (mirrors CI 'cli-tests' job)
	@echo "=== CLI Tests (mirrors CI 'cli-tests' job) ==="
	@echo "Starting master via docker compose..."
	@TEST_ADMIN_PASSWORD=$(TEST_ADMIN_PASS) $(COMPOSE) -f docker/docker-compose.test.yml up -d master
	@echo "Waiting for master to be ready..."
	@for i in $$(seq 1 30); do \
		if curl -sf $(TEST_HTTP_URL)/api/v1/health >/dev/null 2>&1; then \
			echo "Master is ready!"; \
			break; \
		fi; \
		echo "Attempt $$i/30: Waiting..."; \
		sleep 2; \
	done
	@curl -sf $(TEST_HTTP_URL)/api/v1/health >/dev/null || \
		($(COMPOSE) -f docker/docker-compose.test.yml logs master && $(COMPOSE) -f docker/docker-compose.test.yml down -v && exit 1)
	@API_TOKEN=$$($(MAKE) -s _create-api-token) && \
	echo "Running CLI tests..." && \
	VCDEPLOY_BINARY=./$(OUT_DIR)/vcdeploy \
		E2E_MASTER_HTTP_URL=$(TEST_HTTP_URL) E2E_API_TOKEN=$$API_TOKEN \
		E2E_ADMIN_PASS=$(TEST_ADMIN_PASS) SKIP_AGENT_TESTS=1 \
		go test -v -tags=cli -timeout $(TEST_TIMEOUT) -p=1 ./tests/cli/... ; \
	TEST_EXIT=$$?; \
	$(COMPOSE) -f docker/docker-compose.test.yml down -v; \
	exit $$TEST_EXIT

# ============================================================================
# Testing - UI Tests (Starts own server process)
# ============================================================================

.PHONY: test-ui
test-ui: build ## Run Playwright UI tests (mirrors CI 'ui-tests' job)
	@echo "=== UI Tests (mirrors CI 'ui-tests' job) ==="
	@cd tests/ui && npm ci && npx playwright install --with-deps chromium
	@$(MAKE) -s _start-test-server
	@cd tests/ui && npx playwright test --workers=1; \
	TEST_EXIT=$$?; \
	$(MAKE) -s _stop-test-server; \
	exit $$TEST_EXIT

# ============================================================================
# Testing - Run All (mirrors CI pipeline)
# ============================================================================

.PHONY: test-all
test-all: test test-systemd test-integration test-e2e test-cli ## Run all tests (mirrors full CI)
	@echo "=== All tests passed! ==="

# ============================================================================
# Testing - Manual Infrastructure (for debugging)
# ============================================================================

.PHONY: test-infra-up
test-infra-up: ## Start full test infrastructure (for debugging)
	TEST_ADMIN_PASSWORD=$(TEST_ADMIN_PASS) $(COMPOSE) -f docker/docker-compose.test.yml up -d --build --wait
	@echo "Test infrastructure ready!"
	@echo "  Master HTTP: $(TEST_HTTP_URL)"
	@echo "  Master gRPC: localhost:$(TEST_GRPC_PORT)"

.PHONY: test-infra-down
test-infra-down: ## Stop test infrastructure
	$(COMPOSE) -f docker/docker-compose.test.yml down -v

.PHONY: test-infra-logs
test-infra-logs: ## Show test infrastructure logs
	$(COMPOSE) -f docker/docker-compose.test.yml logs -f

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

# Start test server process (for e2e and ui tests)
_start-test-server:
	@echo "Starting test server..."
	@mkdir -p data
	@[ -L data/templates ] || ln -sf ../web/templates data/templates
	@[ -L data/static ] || ln -sf ../web/static data/static
	@VCDEPLOY_TEST_MODE=true VCDEPLOY_ADMIN_PASSWORD=$(TEST_ADMIN_PASS) \
		VCDEPLOY_DATA_DIR=./data VCDEPLOY_CONFIG_DIR=./configs \
		VCDEPLOY_RUN_DIR=./data VCDEPLOY_LOG_DIR=./data \
		./$(OUT_DIR)/vcdeploy master start --config=configs/master-dev.yaml &
	@echo "Waiting for server to be ready..."
	@for i in $$(seq 1 30); do \
		if curl -sf $(TEST_HTTP_URL)/api/v1/health >/dev/null 2>&1; then \
			echo "Server is ready!"; \
			break; \
		fi; \
		sleep 2; \
	done
	@curl -sf $(TEST_HTTP_URL)/api/v1/health >/dev/null || (echo "Server failed to start"; exit 1)

# Stop test server process
_stop-test-server:
	@echo "Stopping test server..."
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
	TOKEN=$$(curl -sf -X POST $(TEST_HTTP_URL)/api/v1/apikeys \
		-H "Content-Type: application/json" \
		-H "Cookie: session=$$SESSION" \
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
