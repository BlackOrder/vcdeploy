# vcdeploy UX Documentation

> **Purpose:** Authoritative reference for API, CLI, and UI design decisions.  
> **Audience:** Developers, contributors, and maintainers.

This directory contains the canonical UX maps for vcdeploy. All user-facing surfaces (API, CLI, UI) must conform to these specifications.

## Documents

| File | Description |
|------|-------------|
| [api-map.md](api-map.md) | REST API endpoint reference — routes, methods, request/response schemas |
| [cli-map.md](cli-map.md) | CLI command tree — commands, flags, help text, examples |
| [ui-map.md](ui-map.md) | Web UI pages — views, API dependencies, interactions |
| [remediation-plan.md](remediation-plan.md) | Implementation plan for UX improvements |

## Design Principles

### 1. REST API

- **Resource-oriented**: URLs represent resources, HTTP methods represent actions
- **Kebab-case**: All URL segments use kebab-case (`/api-keys`, not `/apikeys`)
- **Complete CRUD**: Every resource supports GET (list), POST (create), GET/{id} (show), PUT/{id} (update), DELETE/{id} (delete) where semantically appropriate
- **Consistent responses**: All endpoints return JSON with consistent error format
- **Idempotent operations**: PUT and DELETE are idempotent; POST creates new resources

### 2. CLI

- **Noun-verb structure**: `vcdeploy <resource> <action>` (e.g., `vcdeploy project create`)
- **Consistent verbs**: `list`, `show`, `create`, `update`, `delete` — never `add`, `remove`, `get`, `status`
- **Idiomatic Cobra**: Use `Example` field for examples, `Long` for detailed help, `Aliases` for shortcuts
- **Discoverability**: Every command has comprehensive `--help` with examples
- **Completions**: Full shell completion support (bash, zsh, fish, powershell)

### 3. UI

- **API-driven**: All UI operations go through the REST API — no server-side rendering of data
- **Real-time updates**: Use SSE for live data (logs, status, events)
- **Bulk operations**: Support multi-select and bulk actions on all list views
- **Consistent navigation**: Resource pages follow the same layout pattern

## Coverage Matrix

Every operation must be accessible from all three surfaces:

| Operation | API | CLI | UI |
|-----------|-----|-----|-----|
| CRUD operations | ✅ | ✅ | ✅ |
| Bulk operations | ✅ `/bulk` | ✅ `--bulk` | ✅ Multi-select |
| Real-time logs | ✅ SSE | ✅ `--follow` | ✅ Live stream |
| Export/Import | ✅ JSON | ✅ `--format` | ✅ Download/Upload |

## Versioning

- API version: `v1` (in URL path)
- CLI version: Matches release version
- Breaking changes require API version bump
