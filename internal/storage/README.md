# Storage Package

The storage package provides a unified storage layer for vcdeploy with support for both SQLite-backed and in-memory storage.

## Architecture

### Store Interface

The `Store` interface (`interface.go`) defines all storage operations. Both `DB` (SQLite) and `MemoryStore` implement this interface, allowing services to work with either backend transparently.

```go
type Store interface {
    // Connection and lifecycle
    Conn() *sql.DB
    Close() error
    
    // Entity operations (users, projects, agents, etc.)
    // See interface.go for the complete API
}
```

### MemoryStore

`MemoryStore` provides a fast, in-memory storage implementation with optional background persistence to SQLite.

Key features:
- **Fast reads**: All data is in memory with O(1) lookups
- **Thread-safe**: RWMutex protection for all operations
- **Batched writes**: Background goroutines batch SQLite writes for persistence
- **Copy semantics**: Copy-on-store and copy-on-read prevent external mutation

```go
// Create a memory-only store
store := storage.NewMemoryStore(nil)

// Create a memory store with SQLite persistence
db, _ := storage.NewDB("vcdeploy.db", logger)
store := storage.NewMemoryStore(db)
if err := store.LoadFromDB(ctx); err != nil {
    // Handle error
}
```

### Write Operations

Write operations are batched and sent to SQLite in the background via typed channels:

```go
type WriteOp struct {
    Type      WriteOpType  // WriteOpInsert, WriteOpUpdate, WriteOpDelete
    Entity    string       // "users", "projects", etc.
    ID        interface{}  // Entity ID
    Data      interface{}  // Entity data
    Timestamp time.Time    // Operation timestamp
}
```

Seven separate write channels handle different entity categories for optimized batching:
- `coreWrites`: Users, sessions, API keys
- `projectsWrites`: Projects, webhooks, secrets
- `agentsWrites`: Agents, agent binaries
- `deploymentsWrites`: Deployments, logs, rollbacks, scheduled
- `auditWrites`: Audit logs
- `ratelimitWrites`: Rate limit records, blocked IPs
- `provisionWrites`: Provision jobs, host keys, jump servers

## Testing

### Test Helpers

Use `TestMemoryStore` for tests:

```go
func TestSomething(t *testing.T) {
    store := storage.NewTestMemoryStore(t)
    
    // Use Must* methods for test setup
    user := store.MustCreateUser("admin", "admin@example.com", "hash")
    project := store.MustCreateProject("myproject")
    
    // Or seed comprehensive test data
    data := store.SeedTestData()
    // data.AdminUser, data.Project1, etc.
}
```

### Running Tests

```bash
# Run all storage tests
go test ./internal/storage/...

# Run with verbose output
go test -v ./internal/storage/...

# Run specific tests
go test -v ./internal/storage/... -run TestMemoryStore_Users
```

## Migration Guide

### Service Migration

Services should use `storage.Store` interface instead of `*storage.DB`:

```go
// Before
type Service struct {
    db *storage.DB
}

func New(db *storage.DB) *Service {
    return &Service{db: db}
}

// After
type Service struct {
    store storage.Store
}

func New(store storage.Store) *Service {
    return &Service{store: store}
}
```

### Loading from Existing Database

To initialize a MemoryStore from an existing SQLite database:

```go
db, err := storage.NewDB(dbPath, logger)
if err != nil {
    return err
}

store := storage.NewMemoryStore(db)
if err := store.LoadFromDB(ctx); err != nil {
    return err
}

// Now use store for all operations
// Reads are from memory, writes are batched to SQLite
```

## File Structure

| File | Description |
|------|-------------|
| `interface.go` | Store interface definition |
| `db.go` | SQLite DB implementation |
| `models.go` | Entity structs (User, Project, etc.) |
| `memory.go` | MemoryStore core implementation |
| `memory_users.go` | User, session, API key operations |
| `memory_projects.go` | Project, webhook, secret operations |
| `memory_agents.go` | Agent, binary operations |
| `memory_deployments.go` | Deployment, log, rollback operations |
| `memory_audit.go` | Audit, settings operations |
| `memory_ratelimit.go` | Rate limiting operations |
| `memory_misc.go` | Miscellaneous entity operations |
| `memory_cleanup.go` | Cleanup and maintenance operations |
| `memory_loader.go` | LoadFromDB implementation |
| `writeop.go` | Write operation types and batch processing |
| `test_helpers.go` | Test utilities and fixtures |

## Error Handling

Standard errors are defined for consistent error handling:

```go
var (
    ErrNotFound       = errors.New("not found")
    ErrDuplicate      = errors.New("duplicate entry")
    ErrValidation     = errors.New("validation error")
    ErrNotImplemented = errors.New("not implemented")
)
```

Services should check for these errors:

```go
user, err := store.GetUserByID(ctx, id)
if errors.Is(err, storage.ErrNotFound) {
    // Handle not found
}
```
