# Changelog

All notable changes to vcdeploy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **API Pagination**: Users, Projects, and Agents list endpoints now support pagination
  - Query parameters: `page` (1-indexed), `pageSize` (default: 20, max: 100)
  - Response includes: `items`, `total`, `page`, `pageSize`, `hasMore`
- **OpenAPI Documentation**: Enhanced OpenAPI spec with pagination schemas, proper error types, and examples
- **Service Layer Documentation**: Comprehensive godoc for services/interfaces.go

### Changed
- **API Response Consistency**:
  - DELETE endpoints now return 200 with JSON `{"status": "deleted"}` instead of 204 No Content
  - POST for creating resources returns 201 Created consistently
  - All error responses use consistent `{"error": true, "message": "..."}` format
- **Error Handling**:
  - `http.Error` calls replaced with `jsonError` helper for JSON responses
  - All API errors now return proper JSON with consistent structure
- **Test Improvements**:
  - UI tests: Replaced `waitForTimeout` with proper wait conditions
  - E2E tests: Replaced `NoServerError` assertions with specific status checks
  - Fixed always-true assertions in UI specs
  - Added conflict logging to test seeder for debugging

### Fixed
- **Critical**: Added error handling for `GetSystemConfig()` calls that previously panicked
- **Critical**: Added fallback for `crypto/rand.Read` errors using timestamp-based seed
- **Critical**: Enhanced audit logging with full context on authorization failures
- **Security**: Deprecated `MustValidate` command validator in favor of error-returning version

### Deprecated
- `MustValidate` in cmdvalidator.go - use `Validate` instead (will panic on invalid commands)
- `MustGetSystemConfig` in paths.go - use `GetSystemConfig` which returns errors

### Security
- All gosec suppressions reviewed and verified with justifications
- No new security issues introduced

## Pre-Release Notes

vcdeploy is currently in pre-release development. This changelog tracks changes
during the comprehensive audit remediation process.

### Audit Summary (47 Issues Addressed)
- Critical (5): Panic code paths, ignored errors, fallback handling
- High (15): API consistency, pagination, error standardization
- Medium (17): Documentation, validation, test improvements
- Low (10): Code quality, suppressions review

[Unreleased]: https://github.com/BlackOrder/vcdeploy/compare/main...HEAD
