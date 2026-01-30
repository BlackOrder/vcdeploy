# VCDeploy Makefile
# ==================
# Development-only targets. GoReleaser handles all release artifacts.

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

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

# Test flags
TEST_FLAGS   := -v -race
TEST_TIMEOUT := 10m

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
clean: ## Remove build artifacts
	rm -rf $(OUT_DIR) dist/ coverage.out coverage.html
	go clean -testcache

# ============================================================================
# Development
# ============================================================================

.PHONY: dev
dev: build ## Run master in development mode
	@mkdir -p data
	@[ -L data/templates ] || ln -sf ../web/templates data/templates
	@[ -L data/static ] || ln -sf ../web/static data/static
	VCDEPLOY_DATA_DIR=./data VCDEPLOY_CONFIG_DIR=./configs VCDEPLOY_RUN_DIR=./data VCDEPLOY_LOG_DIR=./data \
		./$(OUT_DIR)/vcdeploy master start --config=configs/master-dev.yaml

.PHONY: dev-agent
dev-agent: build ## Run agent in development mode
	./$(OUT_DIR)/vcdeploy-agent start --config=configs/agent.yaml.example

# ============================================================================
# Testing
# ============================================================================

.PHONY: test
test: ## Run unit tests
	go test -short -race ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: test-integration
test-integration: ## Run integration tests (requires Docker)
	go test -race -tags=integration -timeout $(TEST_TIMEOUT) ./...

.PHONY: test-systemd
test-systemd: ## Run systemd unit validation tests
	go test -v -tags=systemd ./init/...

.PHONY: test-e2e
test-e2e: docker-test-up ## Run E2E tests
	go test -race -tags=e2e -timeout $(TEST_TIMEOUT) ./tests/e2e/...

.PHONY: test-cli
test-cli: ## Run CLI tests (requires running infrastructure)
	go test -v -tags=cli -timeout $(TEST_TIMEOUT) ./tests/cli/...

.PHONY: test-ui
test-ui: ## Run Playwright UI tests
	cd tests/ui && npm ci && npx playwright test --workers=1

.PHONY: test-all
test-all: test test-integration test-systemd ## Run all tests except E2E

.PHONY: test-bench
test-bench: ## Run benchmarks
	go test -bench=. -benchmem -run=^$$ ./...

# ============================================================================
# Code Quality
# ============================================================================

.PHONY: lint
lint: ## Run linter
	golangci-lint run ./...

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

.PHONY: docker-test-up
docker-test-up: ## Start test infrastructure
	$(COMPOSE) -f docker/docker-compose.test.yml up -d --build --wait
	@echo "Test infrastructure ready!"
	@echo "  Master HTTP: http://localhost:8080"
	@echo "  Master gRPC: localhost:9090"

.PHONY: docker-test-down
docker-test-down: ## Stop test infrastructure
	$(COMPOSE) -f docker/docker-compose.test.yml down -v

.PHONY: docker-logs
docker-logs: ## Show container logs
	$(COMPOSE) -f docker/docker-compose.yml logs -f

# ============================================================================
# Release (GoReleaser)
# ============================================================================

.PHONY: release-snapshot
release-snapshot: ## Build snapshot release (for testing)
	goreleaser release --snapshot --clean

.PHONY: release-check
release-check: ## Validate GoReleaser config
	goreleaser check
