# Changelog

All notable changes to vcdeploy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Prometheus metrics endpoint (`/metrics`) with comprehensive instrumentation
- Kubernetes-style health endpoints (`/healthz`, `/livez`, `/readyz`)
- Request ID middleware for request correlation (X-Request-ID header)
- Detailed health status endpoint with dependency checks
- FreeBSD support (amd64, arm64)
- OpenRC init scripts for Alpine/Gentoo
- FreeBSD rc.d scripts
- macOS launchd plists for agent service
- Multi-architecture Docker images (amd64, arm64)

### Changed
- Migrated to GoReleaser v2 for all release automation
- Simplified Makefile to development-only targets
- Standardized on Go 1.25
- Unix-only focus (removed Windows targets)

### Removed
- Windows build targets
- Manual package build scripts (now handled by GoReleaser)

## [0.1.0] - 2025-01-01

### Added
- Initial release
- Master server with REST API and Web UI
- Agent with gRPC streaming connection
- Project-based deployment configuration
- Secret management with AES-256-GCM encryption
- Audit logging
- Webhook support (GitHub, GitLab, Bitbucket)
- SQLite database storage
- systemd service files
- Docker support
