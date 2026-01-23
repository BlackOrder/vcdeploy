.PHONY: all build build-master build-agent clean test lint proto install help

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Output directory
OUT_DIR := bin

# Proto files
PROTO_DIR := api/proto
PROTO_OUT := internal/proto

all: build

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

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(OUT_DIR)
	@rm -rf $(PROTO_OUT)/*.pb.go

## test: Run tests
test:
	@echo "Running tests..."
	go test -v -race ./...

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## lint: Run linter
lint:
	@echo "Running linter..."
	golangci-lint run ./...

## proto: Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@mkdir -p $(PROTO_OUT)
	protoc --go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/*.proto

## install: Install binaries to /usr/local/bin
install: build
	@echo "Installing binaries..."
	@sudo cp $(OUT_DIR)/vcdeploy /usr/local/bin/
	@sudo cp $(OUT_DIR)/vcdeploy-agent /usr/local/bin/
	@echo "Installed to /usr/local/bin/"

## install-systemd: Install systemd service files
install-systemd:
	@echo "Installing systemd services..."
	@sudo cp init/vcdeploy-master.service /etc/systemd/system/
	@sudo cp init/vcdeploy-agent.service /etc/systemd/system/
	@sudo systemctl daemon-reload
	@echo "Systemd services installed. Enable with:"
	@echo "  sudo systemctl enable vcdeploy-master"
	@echo "  sudo systemctl enable vcdeploy-agent"

## dev: Run master in development mode
dev:
	@echo "Running master in development mode..."
	go run ./cmd/vcdeploy master start --config=configs/master.yaml.example

## dev-agent: Run agent in development mode
dev-agent:
	@echo "Running agent in development mode..."
	go run ./cmd/vcdeploy-agent start --config=configs/agent.yaml.example

## help: Show this help
help:
	@echo "vcdeploy - Deployment Platform"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
