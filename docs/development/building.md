# Building from Source

Build vcdeploy from source for development or custom builds.

## Prerequisites

- **Go 1.25+**: [Download](https://golang.org/dl/)
- **Git**: For cloning the repository
- **Make**: For build automation
- **Node.js 20+**: For web UI (optional)

## Quick Build

```bash
# Clone repository
git clone https://github.com/BlackOrder/vcdeploy.git
cd vcdeploy

# Build binaries
make dev-build

# Binaries are in bin/
./bin/vcdeploy version
./bin/vcdeploy-agent --version
```

## Build Options

### Development Build

```bash
# Fast build without optimizations
make dev-build
```

### Production Build

```bash
# Optimized build with version info
make build VERSION=1.0.0
```

### Cross-Compilation

```bash
# Build for Linux AMD64
GOOS=linux GOARCH=amd64 go build -o vcdeploy-linux-amd64 ./cmd/vcdeploy

# Build for macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o vcdeploy-darwin-arm64 ./cmd/vcdeploy

# Build for FreeBSD
GOOS=freebsd GOARCH=amd64 go build -o vcdeploy-freebsd-amd64 ./cmd/vcdeploy
```

### Supported Platforms

| OS | Architecture |
|----|--------------|
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |
| FreeBSD | amd64, arm64 |

## Build with Docker

```bash
# Build using Docker (no Go installation needed)
docker run --rm -v $(pwd):/app -w /app golang:1.25 make dev-build
```

## GoReleaser

For release builds, use GoReleaser:

```bash
# Install GoReleaser
go install github.com/goreleaser/goreleaser/v2@latest

# Build snapshot (no publish)
goreleaser release --snapshot --clean

# Artifacts are in dist/
```

GoReleaser produces:
- Binaries for all platforms
- Docker images
- deb/rpm packages
- Homebrew formula

## Web UI

The web UI is built with Svelte:

```bash
cd web

# Install dependencies
npm install

# Development server
npm run dev

# Production build
npm run build
```

The built UI is embedded in the Go binary.

## Build Flags

### Version Information

```bash
go build -ldflags "-X main.version=1.0.0 -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" ./cmd/vcdeploy
```

### Static Linking

For fully static binaries:

```bash
CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' ./cmd/vcdeploy
```

### Size Optimization

```bash
go build -ldflags "-s -w" ./cmd/vcdeploy
# -s: omit symbol table
# -w: omit DWARF debugging info
```

## Verifying the Build

```bash
# Check version
./bin/vcdeploy version

# Run tests
make test

# Run linter
make lint

# Check security
make vuln
```

## Dependencies

View and update dependencies:

```bash
# List dependencies
go list -m all

# Update dependencies
go get -u ./...
go mod tidy

# Check for vulnerabilities
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## Troubleshooting

### "go: command not found"

Install Go from https://golang.org/dl/ and add to PATH:
```bash
export PATH=$PATH:/usr/local/go/bin
```

### CGO Errors

Some systems require CGO. To build without CGO:
```bash
CGO_ENABLED=0 make dev-build
```

### Permission Denied

Ensure the binary has execute permission:
```bash
chmod +x ./bin/vcdeploy
```
