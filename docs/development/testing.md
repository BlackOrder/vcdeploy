# Testing Guide

Comprehensive guide to testing vcdeploy.

## Test Categories

| Type | Location | Command |
|------|----------|---------|
| Unit | `*_test.go` files | `make test` |
| Integration | `tests/integration/` | `make test-integration` |
| E2E | `tests/e2e/` | `make test-e2e` |
| CLI | `tests/cli/` | `go test -tags=cli ./tests/cli/...` |

## Running Tests

### Unit Tests

```bash
# Run all unit tests
make test

# Run with verbose output
go test -v ./...

# Run specific package
go test -v ./internal/server/...

# Run specific test
go test -v -run TestDeployment ./internal/deploy/...
```

### With Coverage

```bash
# Generate coverage report
make test-coverage

# View HTML report
open coverage.html
```

### Race Detection

```bash
go test -race ./...
```

## Integration Tests

Integration tests require Docker:

```bash
# Start test infrastructure
make docker-test-up

# Run integration tests
make test-integration

# Stop infrastructure
make docker-test-down
```

### Test Infrastructure

```yaml
# docker/docker-compose.test.yml
services:
  ssh:
    image: linuxserver/openssh-server
    # SSH server for deployment tests
  
  master:
    build: .
    # vcdeploy master for E2E tests
```

## Writing Tests

### Unit Test Structure

```go
func TestDeployment_Execute(t *testing.T) {
    // Arrange
    deployer := NewDeployer(config)
    project := &Project{Name: "test"}
    
    // Act
    result, err := deployer.Execute(context.Background(), project)
    
    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.Status != "success" {
        t.Errorf("expected success, got %s", result.Status)
    }
}
```

### Table-Driven Tests

```go
func TestValidate(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    bool
        wantErr bool
    }{
        {"valid", "test", true, false},
        {"empty", "", false, true},
        {"spaces", "  ", false, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Validate(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("Validate() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Using Mocks

```go
// internal/testutil/mocks/storage.go
type MockStorage struct {
    GetProjectFunc func(id string) (*Project, error)
}

func (m *MockStorage) GetProject(id string) (*Project, error) {
    if m.GetProjectFunc != nil {
        return m.GetProjectFunc(id)
    }
    return nil, errors.New("not implemented")
}

// In tests
func TestHandler_GetProject(t *testing.T) {
    mockStorage := &mocks.MockStorage{
        GetProjectFunc: func(id string) (*Project, error) {
            return &Project{ID: id, Name: "test"}, nil
        },
    }
    
    handler := NewHandler(mockStorage)
    // ... test handler
}
```

## Test Fixtures

```go
// tests/testutil/fixtures/fixtures.go
func CreateTestProject(t *testing.T) *storage.Project {
    t.Helper()
    return &storage.Project{
        ID:   "test-project",
        Name: "Test Project",
        // ...
    }
}

// Usage
func TestDeployment(t *testing.T) {
    project := fixtures.CreateTestProject(t)
    // ...
}
```

## Test Database

```go
func TestWithDatabase(t *testing.T) {
    // Create temporary database
    db, err := storage.NewStorage(":memory:")
    if err != nil {
        t.Fatalf("failed to create test db: %v", err)
    }
    defer db.Close()
    
    // Run migrations
    if err := db.Migrate(); err != nil {
        t.Fatalf("failed to migrate: %v", err)
    }
    
    // Test...
}
```

## HTTP Handler Tests

```go
func TestHealthHandler(t *testing.T) {
    // Create request
    req := httptest.NewRequest("GET", "/healthz", nil)
    w := httptest.NewRecorder()
    
    // Create handler
    server := NewServer(config)
    server.ServeHTTP(w, req)
    
    // Assert
    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
}
```

## Benchmarks

```go
func BenchmarkEncrypt(b *testing.B) {
    key := []byte("32-byte-key-for-aes-256-encrypt!")
    data := []byte("test data to encrypt")
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        Encrypt(key, data)
    }
}
```

Run benchmarks:
```bash
go test -bench=. -benchmem ./internal/security/...
```

## Coverage Requirements

Current target: 65%

```bash
# Check coverage
make test-coverage
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
echo "Coverage: $COVERAGE%"
```

## CI Testing

Tests run automatically on:
- Push to main/develop
- Pull requests
- Daily scheduled runs

### GitHub Actions

```yaml
# Runs on every PR
- name: Test
  run: make test

- name: Check Coverage
  run: |
    make test-coverage
    # Fail if below threshold
```
