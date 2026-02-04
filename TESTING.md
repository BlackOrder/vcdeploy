# Testing Guide for vcdeploy

This document describes the testing strategy, structure, and best practices for the vcdeploy project.

## Table of Contents

- [Test Categories](#test-categories)
- [Running Tests](#running-tests)
- [Test Structure](#test-structure)
- [Writing Tests](#writing-tests)
- [CI/CD Pipeline](#cicd-pipeline)
- [Coverage](#coverage)

## Test Categories

### Unit Tests
Fast, isolated tests that run without external dependencies. They test individual functions, methods, and packages in isolation.

**Location**: `*_test.go` files throughout the codebase
**Build tag**: None (default)
**Run time**: < 1 minute

### Integration Tests
Tests that require external services like Docker containers (SSH servers, databases). Uses testcontainers-go for spinning up real services.

**Location**: Files with `//go:build integration`
**Build tag**: `integration`
**Run time**: 2-5 minutes

### Systemd Tests
Tests that validate systemd unit files for syntax, dependencies, and configuration.

**Location**: `init/systemd_test.go`
**Build tag**: `systemd`
**Requirements**: `systemd-analyze` command available

### End-to-End Tests
Full system tests that run the complete master-agent architecture using docker-compose.

**Location**: `tests/e2e/`
**Build tag**: `e2e`
**Requirements**: Docker, docker-compose

## Running Tests

### Quick Reference

```bash
# Run unit tests only (fast)
make test

# Run all tests including integration and systemd
make test-all

# Run integration tests (requires Docker)
make test-integration

# Run systemd validation tests
make test-systemd

# Run e2e tests (requires docker-compose)
make test-e2e

# Run tests with coverage report
make test-coverage

# Run benchmarks
make test-bench
```

### Direct Go Commands

```bash
# Unit tests with verbose output
go test -v -race ./...

# Unit tests (short mode, skips slow tests)
go test -v -race -short ./...

# Integration tests
go test -v -race -tags=integration ./...

# Systemd tests
go test -v -tags=systemd ./init/...

# E2E tests
go test -v -tags=e2e ./tests/e2e/...

# Run specific package
go test -v ./internal/config/...

# Run specific test
go test -v -run TestConfigLoad ./internal/config/...

# Run tests with coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

## Test Structure

```
vcdeploy/
├── internal/
│   ├── agent/
│   │   └── agent_test.go          # Agent lifecycle tests
│   ├── config/
│   │   └── config_test.go         # Configuration loading tests
│   ├── deploy/
│   │   └── executor_test.go       # Deployment executor tests
│   ├── notify/
│   │   └── notify_test.go         # Notification channel tests
│   ├── proto/
│   │   └── agent_test.go          # gRPC service tests
│   ├── security/
│   │   └── security_test.go       # Encryption/security tests
│   ├── server/
│   │   └── master_test.go         # HTTP API handler tests
│   ├── ssh/
│   │   └── ssh_test.go            # SSH client tests
│   ├── storage/
│   │   └── storage_test.go        # SQLite storage tests
│   └── webhooks/
│       └── handler_test.go        # Webhook handler tests
├── cmd/
│   └── vcdeploy/
│       └── commands/
│           ├── context.go          # AppContext for CLI testing
│           └── context_test.go     # CLI command tests
├── init/
│   └── systemd_test.go            # Systemd unit validation
├── tests/
│   └── e2e/
│       ├── api_test.go            # API endpoint E2E tests
│       ├── deploy_test.go         # Deployment E2E tests
│       └── config/                # E2E test configurations
│           ├── master.yaml
│           └── agent.yaml
├── docker-compose.test.yaml       # E2E test infrastructure
└── testutil/
    ├── containers/                # Testcontainers helpers
    │   └── containers.go          # SSH, Git container setup
    ├── fixtures/                  # Test data fixtures
    ├── mocks/                     # Mock implementations
    └── helpers.go                 # Test utilities
```

### Running E2E Tests

E2E tests require Docker and docker-compose. They spin up a complete environment:
- VCDeploy Master
- VCDeploy Agent
- SSH Target Server
- Git Server (Gitea)

```bash
# Start E2E test environment
docker-compose -f docker-compose.test.yaml up -d --build

# Wait for services to be healthy
docker-compose -f docker-compose.test.yaml ps

# Run E2E tests
go test -v -tags=e2e ./tests/e2e/...

# Clean up
docker-compose -f docker-compose.test.yaml down -v
```

Or use the Makefile target:
```bash
make test-e2e
```

## Writing Tests

### Test File Conventions

1. **Naming**: Test files should be named `*_test.go` in the same package
2. **Parallelism**: Always add `t.Parallel()` at the start of each test function
3. **Table-driven**: Use table-driven tests for testing multiple cases
4. **Subtests**: Use `t.Run()` for subtests with descriptive names

### Example Unit Test

```go
package config

import (
    "testing"
)

func TestConfigValidation(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name    string
        config  Config
        wantErr bool
    }{
        {
            name: "valid config",
            config: Config{
                Server: ServerConfig{Port: 8080},
            },
            wantErr: false,
        },
        {
            name: "invalid port",
            config: Config{
                Server: ServerConfig{Port: -1},
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        tt := tt // capture range variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            
            err := tt.config.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Example Integration Test

```go
//go:build integration

package ssh

import (
    "context"
    "testing"
    
    "github.com/testcontainers/testcontainers-go"
)

func TestSSHClient_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    
    ctx := context.Background()
    
    // Start SSH container
    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "linuxserver/openssh-server",
            ExposedPorts: []string{"2222/tcp"},
            // ... configuration
        },
        Started: true,
    })
    if err != nil {
        t.Fatalf("failed to start container: %v", err)
    }
    defer container.Terminate(ctx)
    
    // Run tests against container
    // ...
}
```

### Using Test Utilities

```go
package mypackage

import (
    "testing"
    
    "github.com/BlackOrder/vcdeploy/testutil"
)

func TestWithTempDir(t *testing.T) {
    t.Parallel()
    
    // Create a temp directory that's cleaned up automatically
    tempDir := testutil.TempDir(t)
    
    // Create a test config
    cfg := testutil.TestConfig(t)
    
    // Use mock logger
    logger := testutil.MockLogger()
    
    // ... test implementation
}
```

### Testing HTTP Handlers

```go
func TestAPIHandler(t *testing.T) {
    t.Parallel()
    
    // Create test request
    req := httptest.NewRequest("POST", "/api/deploy", strings.NewReader(`{"project":"test"}`))
    req.Header.Set("Content-Type", "application/json")
    
    // Create response recorder
    rr := httptest.NewRecorder()
    
    // Call handler
    handler := NewDeployHandler(mockDeps)
    handler.ServeHTTP(rr, req)
    
    // Assert response
    if rr.Code != http.StatusOK {
        t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
    }
}
```

### Testing CLI Commands

The project uses an `AppContext` pattern for CLI testability:

```go
func TestVersionCommand(t *testing.T) {
    t.Parallel()
    
    var stdout bytes.Buffer
    ctx := commands.NewAppContext().WithStdout(&stdout)
    
    runner := commands.NewVersionRunner(ctx, "1.0.0", "abc123")
    err := runner.Run()
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    if !strings.Contains(stdout.String(), "1.0.0") {
        t.Errorf("expected version in output, got: %s", stdout.String())
    }
}
```

## CI/CD Pipeline

The GitHub Actions workflow (`.github/workflows/test.yml`) runs:

1. **Unit Tests**: Fast tests on every push/PR
2. **Systemd Tests**: Validates systemd unit files
3. **Integration Tests**: Runs after unit tests pass (uses Docker)
4. **Test Matrix**: Tests across Go 1.23 and 1.24 on Ubuntu
5. **E2E Tests**: Full system tests on main branch only
6. **Benchmarks**: Performance benchmarks on push

### Pipeline Diagram

```
┌─────────────┐
│  Unit Tests │
└──────┬──────┘
       │
       ├─────────────────────────────┐
       │                             │
       ▼                             ▼
┌──────────────────┐    ┌─────────────────────┐
│ Integration Tests│    │    Test Matrix      │
│ (testcontainers) │    │ (Go 1.23, 1.24)     │
└────────┬─────────┘    └──────────┬──────────┘
         │                         │
         └───────────┬─────────────┘
                     │
                     ▼
              ┌────────────┐
              │  E2E Tests │
              │ (main only)│
              └────────────┘
```

## Coverage

### Generating Coverage Reports

```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./...

# View coverage in terminal
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html

# Open in browser
open coverage.html  # macOS
xdg-open coverage.html  # Linux
```

### Coverage Goals

| Package | Target Coverage |
|---------|-----------------|
| `internal/config` | 90%+ |
| `internal/deploy` | 85%+ |
| `internal/ssh` | 80%+ |
| `internal/server` | 85%+ |
| `internal/agent` | 80%+ |
| `internal/security` | 90%+ |
| `internal/storage` | 85%+ |
| `cmd/vcdeploy` | 70%+ |

### Viewing Coverage in CI

Coverage reports are uploaded to Codecov on each push. View the coverage badge and detailed reports at:
- https://codecov.io/gh/BlackOrder/vcdeploy

## Skip Traceability Matrix

This section documents all `t.Skip()` calls in the test suite and explains why each test is skipped and where equivalent coverage exists.

### Skip Categories

| Category | Count | Coverage Location | Notes |
|----------|-------|-------------------|-------|
| OS-specific (Linux) | 1 | CI runs on Linux | `machine_key_test.go:64` - primary platform |
| OS-specific (macOS) | 2 | macOS CI diff-only | `machine_key_test.go:82,100` - UUID retrieval |
| Root privileges | 3 | Integration tests with Docker | `executor_test.go:97,109`, `agent_test.go:767` |
| External services (ACME) | 1 | Manual with staging endpoint | `acme_integration_test.go:36` |
| Database dependencies | 2 | Integration tests | `db_recipes_test.go:276,300` |
| Template loading | 3 | E2E tests with proper setup | `master_test.go:39,42,1180` |
| Network timeout | 1 | Integration tests | `admin_test.go:1825` |
| Consolidated schema | 3 | Single migration design | `migrations_test.go:402,459,511` |
| E2E resource not available | 19 | Dynamic skip when fixture missing | E2E tests create fixtures first |
| ACME environment | 1 | Manual test only | Requires `ACME_TEST_DOMAIN` |
| Component validation | 1 | Integration tests | `resolver_test.go:185` |

### Detailed Skip Explanations

#### Root Privileges (3 skips)
- `cmd/vcdeploy/commands/executor_test.go:97` - Test requires root for file ownership tests
- `cmd/vcdeploy/commands/executor_test.go:109` - Test requires non-root user
- `internal/agent/agent_test.go:767` - Agent restart requires root

**Coverage:** Integration tests run as root in Docker containers.

#### OS-Specific Tests (3 skips)
- `internal/agent/machine_key_test.go:52` - Machine ID not available on all OS
- `internal/agent/machine_key_test.go:64` - Linux-specific `/etc/machine-id`
- `internal/agent/machine_key_test.go:82,100` - macOS-specific `ioreg`

**Coverage:** CI runs full suite on Linux (primary platform). macOS gets diff-only tests.

#### Template Loading (3 skips)
- `internal/server/master_test.go:39,42` - Templates directory not found
- `internal/server/master_test.go:1180` - UI tests need templates loaded

**Coverage:** E2E tests ensure templates are available via docker-compose setup.

#### E2E Dynamic Skips (19 skips)
- `tests/e2e/agents_test.go` - Skip if no agents available
- `tests/e2e/users_test.go` - Skip if no user created
- `tests/e2e/api_extended_test.go` - Skip if master not available

**Coverage:** These are conditional skips that run when fixtures exist. E2E suite creates fixtures before running dependent tests.

#### Database Dependencies (2 skips)
- `internal/storage/db_recipes_test.go:276` - Requires full chain setup
- `internal/storage/db_recipes_test.go:300` - Requires user in database

**Coverage:** Integration tests with full database available.

#### Consolidated Schema Skips (3 skips)
- `migrations_test.go:402,459,511` - Tests for multi-migration scenarios

**Coverage:** These tests are no longer relevant after Stage 1 schema consolidation. The tests remain to document the expected behavior if the schema is ever split again.

### Platform Support

| Platform | Support Level | Test Coverage |
|----------|---------------|---------------|
| Linux | Primary | Full test suite in CI |
| macOS | Secondary | OS-specific diff-only tests |
| Windows | Not Supported | No tests |

**Rationale:**
- Linux is the primary deployment platform (servers)
- macOS needs only OS-specific code paths verified (UUID, system info)
- Windows is not a deployment target

### Coverage Baseline (as of 2025-01-17)

Packages below 50% coverage that need improvement:

| Package | Coverage | Notes |
|---------|----------|-------|
| `cmd/vcdeploy/commands` | 20.0% | CLI commands need more tests |
| `internal/agent` | 38.3% | Agent lifecycle tests needed |
| `internal/git` | 39.5% | Git operations tests needed |
| `internal/proto` | 9.4% | gRPC generated code |
| `internal/provision` | 44.0% | Improved from 23.3% after worker tests |
| `internal/server` | 44.1% | HTTP handlers need more coverage |
| `internal/tracing` | 37.5% | Tracing infrastructure |
| `internal/testutil` | 16.4% | Test utilities (acceptable low coverage) |

Packages with good coverage (80%+):
- `internal/alerting` — 95.8%
- `internal/metrics` — 100%
- `internal/security` — 81.3%
- `internal/services/*` — Most services 80%+
- `internal/validation` — 97.3%
- `internal/webhooks` — 85.3%

## Best Practices

### Do's

✅ Use `t.Parallel()` in all tests for faster execution
✅ Use table-driven tests for multiple test cases
✅ Test error paths, not just happy paths
✅ Use descriptive test names that explain the scenario
✅ Clean up resources with `t.Cleanup()` or `defer`
✅ Use build tags for tests requiring external dependencies
✅ Mock external dependencies in unit tests

### Don'ts

❌ Don't use `init()` in test files
❌ Don't rely on test execution order
❌ Don't use hardcoded ports (use dynamic allocation)
❌ Don't leave test files or directories after tests
❌ Don't test private functions directly (test through public API)
❌ Don't use `time.Sleep()` for synchronization (use channels/conditions)

## Troubleshooting

### Tests Hang or Timeout

1. Check for goroutine leaks using `-race` flag
2. Ensure all channels are properly closed
3. Use context with timeout for external calls

### Integration Tests Fail

1. Ensure Docker is running: `docker ps`
2. Check available disk space
3. Try with `TESTCONTAINERS_RYUK_DISABLED=true`

### Coverage is Low

1. Run coverage on specific package: `go test -cover ./internal/config`
2. Identify untested code: `go tool cover -func=coverage.out | grep -v "100.0%"`
3. Add tests for edge cases and error paths

## Adding New Tests

When adding new functionality:

1. Write tests **before** or **alongside** the implementation
2. Start with unit tests
3. Add integration tests if external services are involved
4. Update this documentation if new patterns are introduced

## Resources

- [Go Testing Documentation](https://pkg.go.dev/testing)
- [testcontainers-go](https://golang.testcontainers.org/)
- [Table-Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [Go Race Detector](https://go.dev/blog/race-detector)
