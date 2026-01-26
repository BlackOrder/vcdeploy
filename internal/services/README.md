# Service Layer Architecture

This package provides the service layer for vcdeploy, implementing a clean separation between HTTP/gRPC handlers and data persistence.

## Design Goals

1. **Single Responsibility**: Each service manages one domain (users, projects, deployments, etc.)
2. **Testability**: Services can be easily mocked via interfaces
3. **Maintainability**: Business logic is centralized in services, not scattered across handlers
4. **Consistency**: All services follow the same patterns for error handling, logging, and context usage
5. **Encapsulation**: Database operations are hidden behind service interfaces
6. **Flexibility**: Services can be extended without modifying handlers

## Package Structure

```
internal/services/
├── errors.go               # Standard errors with constructors
├── interfaces.go           # All service interfaces
├── types.go                # Common types (pagination, filters)
├── validation.go           # Input validation helpers
├── README.md               # This file
├── testutil/
│   ├── db.go               # Test database setup + NewTestLogger
│   └── constants.go        # Test constants for consistent data
├── agents/
│   ├── service.go
│   └── service_test.go
├── apikeys/
│   ├── service.go
│   └── service_test.go
├── audit/
│   ├── service.go
│   └── service_test.go
├── deployments/
│   ├── service.go
│   └── service_test.go
├── hostkeys/
│   └── service.go
├── projects/
│   ├── service.go
│   └── service_test.go
├── projecttypes/
│   └── service.go
├── secrets/
│   ├── service.go
│   └── service_test.go
├── sessions/
│   ├── service.go
│   └── service_test.go
├── settings/
│   ├── service.go
│   └── service_test.go
├── users/
│   ├── service.go
│   └── service_test.go
└── webhooks/
    └── service.go
```

## Interface Pattern

Each service implements an interface defined in `interfaces.go`:

```go
// Example: UserServicer interface
type UserServicer interface {
    Create(ctx context.Context, username, password, email, role string) (*storage.User, error)
    GetByID(ctx context.Context, id int64) (*storage.User, error)
    GetByUsername(ctx context.Context, username string) (*storage.User, error)
    List(ctx context.Context) ([]*storage.User, error)
    Count(ctx context.Context) (int64, error)
    Update(ctx context.Context, user *storage.User) error
    Delete(ctx context.Context, id int64) error
    VerifyPassword(ctx context.Context, username, password string) (*storage.User, error)
    UpdatePassword(ctx context.Context, userID int64, newPassword string) error
    SetTOTP(ctx context.Context, userID int64, secret string, enabled bool) error
}
```

## Service Implementation Pattern

Each service follows this structure:

```go
package users

import (
    "context"
    "fmt"
    
    "github.com/BlackOrder/vcdeploy/internal/services"
    "github.com/BlackOrder/vcdeploy/internal/storage"
)

// Compile-time interface check
var _ services.UserServicer = (*Service)(nil)

// Service handles user management.
type Service struct {
    db     *storage.DB
    logger *zap.Logger  // Optional: for services that need logging
}

// New creates a new users Service.
func New(db *storage.DB) *Service {
    return &Service{db: db}
}

// Create creates a new user.
func (s *Service) Create(ctx context.Context, username, password, email, role string) (*storage.User, error) {
    // 1. Validate inputs
    if username == "" {
        return nil, services.ErrInvalidInput
    }
    
    // 2. Business logic
    // ...
    
    // 3. Persist via storage layer
    if err := s.db.CreateUser(ctx, user); err != nil {
        return nil, fmt.Errorf("creating user: %w", err)
    }
    
    return user, nil
}
```

## Error Handling

Services use the comprehensive error system from `errors.go`:

```go
// Sentinel errors
var (
    ErrNotFound      = errors.New("resource not found")
    ErrDuplicate     = errors.New("resource already exists")
    ErrInvalidInput  = errors.New("invalid input")
    ErrUnauthorized  = errors.New("unauthorized")
    ErrForbidden     = errors.New("forbidden")
    ErrConflict      = errors.New("conflict with current state")
    ErrInternal      = errors.New("internal error")
)

// Error constructors with context
err := services.NotFound("users.GetByID", "user", "123")
err := services.Duplicate("users.Create", "user", "admin")
err := services.InvalidInput("users.Create", "username cannot be empty")
err := services.Internal("users.Create", originalErr)
```

Check error types in handlers:

```go
user, err := s.userService.GetByID(ctx, id)
if services.IsNotFound(err) {
    http.Error(w, "User not found", http.StatusNotFound)
    return
}
if services.IsDuplicate(err) {
    http.Error(w, "User already exists", http.StatusConflict)
    return
}
if services.IsInvalidInput(err) {
    http.Error(w, err.Error(), http.StatusBadRequest)
    return
}
```

## Common Types

Use types from `types.go` for pagination and filtering:

```go
// Pagination with bounds checking
pg := services.NewPagination(limit, offset)

// ListResult with total count
result, err := svc.List(ctx, pg)
fmt.Printf("Got %d of %d items\n", len(result.Items), result.TotalCount)
if result.HasMore() {
    nextPg := result.Pagination.NextPage()
}

// Filters
filter := services.AuditFilter{
    Action:     "create",
    Resource:   "user",
    DateRange:  services.DateRange{From: time.Now().Add(-24*time.Hour)},
    Pagination: services.NewPagination(50, 0),
}
```

## Input Validation

Use helpers from `validation.go`:

```go
if err := services.ValidateUsername(username); err != nil {
    return err
}
if err := services.ValidateEmail(email); err != nil {
    return err
}
if err := services.ValidatePassword(password); err != nil {
    return err
}
if err := services.ValidateRole(role); err != nil {
    return err
}
if err := services.ValidateProjectName(name); err != nil {
    return err
}

// Generic validators
services.ValidateRequired("field", value)
services.ValidateMaxLength("field", value, 100)
services.ValidateOneOf("status", status, []string{"pending", "running", "done"})
```

## Context Usage

All service methods accept `context.Context` as the first parameter:

1. **Cancellation**: Operations respect context cancellation
2. **Timeouts**: Callers can set timeouts via context
3. **Values**: Authentication/authorization info can be passed via context

```go
// Handler sets timeout
ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
defer cancel()

users, err := s.userService.List(ctx)
```

## Using Services in Handlers

```go
// MasterServer has service fields
type MasterServer struct {
    // ...
    userService       services.UserServicer
    projectService    services.ProjectServicer
    deploymentService services.DeploymentServicer
    // ...
}

// Initialize in SetWebhookHandler or similar
s.userService = users.New(s.db)
s.projectService = projects.New(s.db)

// Use in handlers
func (s *MasterServer) handleUsers(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    switch r.Method {
    case http.MethodGet:
        users, err := s.userService.List(ctx)
        if err != nil {
            s.logger.Error("Failed to list users", zap.Error(err))
            http.Error(w, "Internal error", http.StatusInternalServerError)
            return
        }
        s.jsonResponse(w, users)
    // ...
    }
}
```

## Services Requiring KMS

Some services need the KMS for encryption:

```go
// Secrets and Settings services require KMS
s.secretService = secrets.New(s.db, kms)
s.settingsSvc = settings.New(s.db, kms)
s.webhookService = webhooks.New(s.db, kms)
```

## Testing

### Using Test Utilities

The `testutil` package provides helpers for testing:

```go
import (
    "testing"
    "github.com/BlackOrder/vcdeploy/internal/services/testutil"
)

func TestMyService(t *testing.T) {
    // Create test database (automatically cleaned up)
    db, cleanup := testutil.NewTestDB(t)
    defer cleanup()

    // Create no-op logger for tests
    logger := testutil.NewTestLogger(t)

    svc := myservice.New(db)
    
    // Use constants for consistent test data
    user := &storage.User{
        Username: testutil.TestUsername,
        Email:    testutil.TestEmail,
        Role:     testutil.TestRole,
    }
}
```

### Test Helpers

```go
// Database helpers
testutil.NewTestDB(t)     // Creates temp SQLite DB for tests
testutil.SetupBenchDB(b)  // Creates temp SQLite DB for benchmarks
testutil.NewTestLogger(t) // Returns zap.NewNop() logger

// Constants for consistent test data
testutil.TestTimeout          // 30 * time.Second
testutil.TestPassword         // "StrongP@ss123!"
testutil.TestWeakPassword     // "weak"
testutil.TestEmail            // "test@example.com"
testutil.TestUsername         // "testuser"
testutil.TestRole             // "user"
testutil.TestAdminRole        // "admin"
testutil.TestProjectName      // "test-project"
testutil.TestRepository       // "https://github.com/example/repo.git"
testutil.TestBranch           // "main"
testutil.TestDeployPath       // "/var/www/test"
testutil.TestAgentID          // "agent-test-001"
testutil.TestHostname         // "test-host"
testutil.TestIPAddress        // "127.0.0.1"
testutil.TestUserAgent        // "Test Agent/1.0"
testutil.TestTOTPSecret       // Sample TOTP secret
testutil.TestAPIKeyPrefix     // "vc_test_"
testutil.TestWebhookSecret    // "webhook-secret-123"
testutil.TestSessionDuration  // 24 * time.Hour
testutil.TestAPIKeyDuration   // 720 * time.Hour (30 days)
```

### Mocking Services

Services can be mocked for unit testing handlers:

```go
// Mock implementation
type mockUserService struct {
    users []*storage.User
}

func (m *mockUserService) List(ctx context.Context) ([]*storage.User, error) {
    return m.users, nil
}

// Use in test
func TestHandleUsers(t *testing.T) {
    server := &MasterServer{
        userService: &mockUserService{
            users: []*storage.User{
                {ID: 1, Username: "admin"},
            },
        },
    }
    // ...
}
```

### Running Tests

```bash
# Run all service tests
go test ./internal/services/...

# Run with coverage
go test -cover ./internal/services/...

# Run specific service tests
go test -v ./internal/services/users/...


# Run with race detector
go test -race ./internal/services/...
```

## Adding a New Service

1. **Define interface** in `interfaces.go`:
   ```go
   type MyServicer interface {
       Create(ctx context.Context, ...) error
       Get(ctx context.Context, id string) (*MyModel, error)
       // ...
   }
   ```

2. **Create package** `internal/services/myservice/service.go`:
   ```go
   package myservice
   
   var _ services.MyServicer = (*Service)(nil)
   
   type Service struct {
       db *storage.DB
   }
   
   func New(db *storage.DB) *Service {
       return &Service{db: db}
   }
   ```

3. **Add to MasterServer**:
   ```go
   type MasterServer struct {
       // ...
       myService services.MyServicer
   }
   ```

4. **Initialize** in `SetWebhookHandler`:
   ```go
   s.myService = myservice.New(s.db)
   ```

5. **Write tests** in `internal/services/myservice/service_test.go`

## Migration from Direct DB Calls

When migrating handlers from direct `s.db.*` calls to services:

1. Identify all `s.db.SomeMethod()` calls in the handler
2. Map to appropriate service method (create service method if needed)
3. Replace `s.db.SomeMethod()` with `s.someService.SomeMethod()`
4. Update error handling (services may return different errors)
5. Run tests to verify behavior

Example migration:
```go
// Before
user, err := s.db.GetUserByUsername(ctx, username)

// After  
user, err := s.userService.GetByUsername(ctx, username)
```
## Interface Design Notes

### Current Interface Sizes

The service interfaces are designed to be cohesive - grouping related operations together:

| Interface | Methods | Responsibility |
|-----------|---------|---------------|
| `UserServicer` | 11 | User CRUD + auth operations |
| `SessionServicer` | 6 | Session management |
| `APIKeyServicer` | 6 | API key management |
| `ProjectServicer` | 6 | Project CRUD |
| `DeploymentServicer` | 14 | Deployments, logs, scheduling, cleanup |
| `SettingsServicer` | 16 | Settings get/set/list |
| `SecretServicer` | 10 | Secret management + import/export |
| `AgentServicer` | 7 | Agent management |
| `AuditServicer` | 3 | Audit logging |
| `WebhookServicer` | 5 | Webhook configuration |
| `HostKeyServicer` | 7 | SSH host key management |
| `ProjectTypeServicer` | 5 | Project type management |
| `RateLimitServicer` | 8 | Rate limiting + IP blocking |
| `ProvisionServicer` | 7 | Agent provisioning jobs |

### Interface Segregation Considerations

The larger interfaces (`DeploymentServicer`, `SettingsServicer`) could potentially be
split into smaller, more focused interfaces. However, the current design:

1. **Maintains Cohesion**: All methods in each interface relate to the same domain
2. **Simplifies Dependency Injection**: Services need one interface, not multiple
3. **Matches Usage Patterns**: Most consumers use multiple methods from the interface
4. **Avoids Over-Engineering**: The interfaces are not unreasonably large

If future requirements dictate, consider splitting:

- `DeploymentServicer` → `DeploymentCRUD`, `DeploymentLogs`, `DeploymentScheduler`, `DeploymentCleaner`
- `SettingsServicer` → `SettingsReader`, `SettingsWriter`, `SettingsManager`

For now, use the existing interfaces and compose behavior as needed.