# Contributing to VCDeploy

Thank you for your interest in contributing to VCDeploy! This document provides guidelines and information for contributors.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Code Style](#code-style)
- [Testing](#testing)
- [Submitting Changes](#submitting-changes)
- [Reporting Issues](#reporting-issues)

## Code of Conduct

This project adheres to a code of conduct. By participating, you are expected to uphold this standard. Please report unacceptable behavior to the maintainers.

## Getting Started

### Prerequisites

- **Go 1.24+** - [Installation Guide](https://golang.org/doc/install)
- **golangci-lint** - `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
- **Make** (optional) - For running common tasks
- **Docker** (optional) - For running integration tests

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/vcdeploy.git
   cd vcdeploy
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/BlackOrder/vcdeploy.git
   ```

## Development Setup

### Install Dependencies

```bash
go mod download
```

### Build

```bash
# Build both binaries
go build -o vcdeploy ./cmd/vcdeploy
go build -o vcdeploy-agent ./cmd/vcdeploy-agent

# Or use go run
go run ./cmd/vcdeploy version
```

### Run Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package tests
go test -v ./internal/services/users/

# Run integration tests (requires Docker)
go test -tags=integration ./tests/integration/
```

### Run Linter

```bash
golangci-lint run ./...
```

## Making Changes

### Branch Naming

Use descriptive branch names:
- `feature/add-webhook-support`
- `fix/session-timeout-bug`
- `docs/update-readme`
- `refactor/service-layer`

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style (formatting, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvement
- `test`: Adding/updating tests
- `ci`: CI/CD changes
- `chore`: Maintenance tasks

Examples:
```
feat(deploy): add rolling deployment strategy
fix(auth): correct session timeout calculation
docs: update installation instructions
test(users): add password validation tests
```

## Code Style

### Go Code

We follow standard Go conventions:

- Use `gofmt` for formatting
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Run `golangci-lint` before committing

Key style points:
- Use meaningful variable names
- Keep functions focused and small
- Add comments for exported functions and types
- Use `context.Context` for cancellation/timeouts
- Handle errors explicitly, don't ignore them

### Error Handling

```go
// Good: Use errors.Is for sentinel error comparison
if errors.Is(err, storage.ErrNotFound) {
    return nil, nil
}

// Good: Use errors.As for type assertions
var exitErr *exec.ExitError
if errors.As(err, &exitErr) {
    return exitErr.ExitCode(), nil
}

// Good: Wrap errors with context
return nil, fmt.Errorf("creating user: %w", err)
```

### Testing

```go
// Use table-driven tests
func TestCreate(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {name: "valid input", input: "test", want: "result"},
        {name: "empty input", input: "", wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Create(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Create() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Testing

### Unit Tests

Write unit tests for all new code. Target 80%+ coverage for service layer code.

```bash
# Run with coverage report
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

### Integration Tests

Integration tests are tagged with `//go:build integration`:

```bash
# Run integration tests
go test -tags=integration -v ./tests/integration/
```

### Benchmark Tests

Add benchmarks for performance-critical code:

```go
func BenchmarkCreate(b *testing.B) {
    // Setup
    db, cleanup := setupTestDB(b)
    defer cleanup()
    svc := New(db)

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        _, _ = svc.Create(context.Background(), "user", "pass", "email", "role")
    }
}
```

Run benchmarks:
```bash
go test -bench=. -benchmem ./internal/services/users/
```

## Submitting Changes

### Pull Request Process

1. **Update your fork**:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Create a branch**:
   ```bash
   git checkout -b feature/your-feature
   ```

3. **Make changes** and commit with conventional commit messages

4. **Run checks locally**:
   ```bash
   golangci-lint run ./...
   go test ./...
   ```

5. **Push and create PR**:
   ```bash
   git push origin feature/your-feature
   ```

6. **Fill out PR template** with:
   - Description of changes
   - Related issue numbers
   - Screenshots (if UI changes)
   - Testing done

### PR Review Checklist

Before requesting review, ensure:

- [ ] All tests pass
- [ ] Linter passes (`golangci-lint run ./...`)
- [ ] New code has tests
- [ ] Documentation is updated
- [ ] Commit messages follow conventions
- [ ] PR description is complete

### After Merge

- Delete your feature branch
- Pull latest changes to local main

## Reporting Issues

### Bug Reports

Include:
1. VCDeploy version (`vcdeploy version`)
2. Operating system and version
3. Steps to reproduce
4. Expected behavior
5. Actual behavior
6. Logs (if applicable)

### Feature Requests

Include:
1. Problem description
2. Proposed solution
3. Alternative solutions considered
4. Additional context

### Security Issues

**Do not report security vulnerabilities publicly.**

Email security issues to: security@blackorder.dev

## Project Structure

```
vcdeploy/
├── cmd/
│   ├── vcdeploy/           # Main CLI binary
│   └── vcdeploy-agent/     # Agent binary
├── internal/
│   ├── agent/              # Agent implementation
│   ├── config/             # Configuration handling
│   ├── deploy/             # Deployment execution
│   ├── notify/             # Notifications
│   ├── proto/              # gRPC protocol buffers
│   ├── provision/          # Server provisioning
│   ├── security/           # Security utilities
│   ├── server/             # Master server
│   ├── services/           # Business logic services
│   ├── storage/            # Database layer
│   └── validation/         # Input validation
├── tests/
│   └── integration/        # Integration tests
├── scripts/                # Build and install scripts
├── init/                   # Service files (systemd, etc.)
└── .github/                # CI/CD workflows
```

## Getting Help

- Open a [GitHub Discussion](https://github.com/BlackOrder/vcdeploy/discussions)
- Check existing [Issues](https://github.com/BlackOrder/vcdeploy/issues)
- Read the [Documentation](https://github.com/BlackOrder/vcdeploy#readme)

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.

---

Thank you for contributing to VCDeploy! 🚀
