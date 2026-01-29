# Contributing

We welcome contributions to vcdeploy!

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR-USERNAME/vcdeploy.git
   cd vcdeploy
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/BlackOrder/vcdeploy.git
   ```

## Development Environment

### Prerequisites

- Go 1.25+
- Node.js 20+ (for web UI)
- Docker (for integration tests)
- Make

### Setup

```bash
# Install dependencies
go mod download

# Build
make dev-build

# Run tests
make test
```

## Code Style

### Go

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofumpt` for formatting
- Run `golangci-lint` before committing

```bash
# Format code
make fmt

# Run linter
make lint
```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `refactor`: Code refactoring
- `test`: Adding tests
- `chore`: Maintenance

Examples:
```
feat(agent): add support for FreeBSD
fix(deploy): handle symlink permission errors
docs(api): document webhook endpoints
```

## Pull Request Process

1. Create a feature branch:
   ```bash
   git checkout -b feat/my-feature
   ```

2. Make your changes

3. Add tests for new functionality

4. Ensure tests pass:
   ```bash
   make test
   make lint
   ```

5. Commit your changes:
   ```bash
   git commit -m "feat(scope): description"
   ```

6. Push to your fork:
   ```bash
   git push origin feat/my-feature
   ```

7. Open a Pull Request

### PR Requirements

- [ ] Tests pass
- [ ] Linting passes
- [ ] Coverage doesn't decrease
- [ ] Documentation updated (if applicable)
- [ ] Commit messages follow convention

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run specific package
go test -v ./internal/server/...

# Run with coverage
make test-coverage
```

### Integration Tests

```bash
# Start test infrastructure
make docker-test-up

# Run integration tests
make test-integration

# Stop test infrastructure
make docker-test-down
```

## Project Structure

```
vcdeploy/
├── cmd/
│   ├── vcdeploy/          # Master CLI
│   └── vcdeploy-agent/    # Agent CLI
├── internal/
│   ├── server/            # HTTP/gRPC server
│   ├── agent/             # Agent implementation
│   ├── storage/           # Database layer
│   ├── deploy/            # Deployment logic
│   ├── config/            # Configuration
│   ├── metrics/           # Prometheus metrics
│   ├── tracing/           # OpenTelemetry tracing
│   └── alerting/          # System alerting
├── docs/                  # Documentation
├── web/                   # Web UI (Svelte)
├── init/                  # Init system scripts
└── tests/                 # Integration/E2E tests
```

## Adding a New Feature

1. **Design**: Discuss in an issue first
2. **Implement**: Follow existing patterns
3. **Test**: Add unit and integration tests
4. **Document**: Update relevant documentation
5. **PR**: Submit with clear description

## Reporting Issues

Include:
- vcdeploy version (`vcdeploy version`)
- OS and architecture
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs

## Code of Conduct

Be respectful and constructive. We're all here to build something great together.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
