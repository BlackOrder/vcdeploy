package security_test

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAuditStore implements only the audit-related parts of storage.Store for testing
type mockAuditStore struct {
	storage.Store
	events []*storage.CertAuditEvent
}

func (m *mockAuditStore) SaveCertAuditEvent(ctx context.Context, event *storage.CertAuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockAuditStore) ListCertAuditEvents(ctx context.Context, filter storage.CertAuditFilter) ([]*storage.CertAuditEvent, error) {
	var result []*storage.CertAuditEvent
	for _, e := range m.events {
		// Apply agent filter
		if filter.AgentID != "" && e.AgentID != filter.AgentID {
			continue
		}
		// Apply since filter
		if filter.Since != nil && e.Timestamp.Before(*filter.Since) {
			continue
		}
		result = append(result, e)
		// Apply limit
		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}
	return result, nil
}

func setupAuditTest(t *testing.T) (*security.CertAuditor, *mockAuditStore) {
	store := &mockAuditStore{events: make([]*storage.CertAuditEvent, 0)}
	auditor := security.NewCertAuditor(store)
	return auditor, store
}

func TestCertAuditor_LogIssued(t *testing.T) {
	auditor, store := setupAuditTest(t)
	ctx := context.Background()

	err := auditor.LogIssued(ctx, "agent-1", "abc123", "ca-1", "user1", "192.168.1.1")
	require.NoError(t, err)

	// Verify event was stored
	require.Len(t, store.events, 1)
	event := store.events[0]
	assert.Equal(t, "agent-1", event.AgentID)
	assert.Equal(t, "abc123", event.Serial)
	assert.Equal(t, storage.CertAuditEventIssued, event.EventType)
	assert.Equal(t, "192.168.1.1", event.ClientIP)
	assert.Equal(t, "ca-1", event.CAID)
	assert.Equal(t, "user1", event.RequestedBy)
}

func TestCertAuditor_LogServerCertIssued(t *testing.T) {
	auditor, store := setupAuditTest(t)
	ctx := context.Background()

	err := auditor.LogServerCertIssued(ctx, "master.example.com", "def456", "ca-1", "system")
	require.NoError(t, err)

	require.Len(t, store.events, 1)
	event := store.events[0]
	assert.Equal(t, "master.example.com", event.Hostname)
	assert.Equal(t, "def456", event.Serial)
	assert.Equal(t, storage.CertAuditEventIssued, event.EventType)
}

func TestCertAuditor_LogRevoked(t *testing.T) {
	auditor, store := setupAuditTest(t)
	ctx := context.Background()

	err := auditor.LogRevoked(ctx, "agent-2", "ghi789", "ca-1", "compromised", "admin")
	require.NoError(t, err)

	require.Len(t, store.events, 1)
	event := store.events[0]
	assert.Equal(t, "agent-2", event.AgentID)
	assert.Equal(t, "ghi789", event.Serial)
	assert.Equal(t, storage.CertAuditEventRevoked, event.EventType)
	assert.Equal(t, "compromised", event.Reason)
	assert.Equal(t, "admin", event.RequestedBy)
}

func TestCertAuditor_LogRenewed(t *testing.T) {
	auditor, store := setupAuditTest(t)
	ctx := context.Background()

	err := auditor.LogRenewed(ctx, "agent-3", "new456", "ca-1", "agent-3", "10.0.0.1")
	require.NoError(t, err)

	require.Len(t, store.events, 1)
	event := store.events[0]
	assert.Equal(t, "agent-3", event.AgentID)
	assert.Equal(t, "new456", event.Serial)
	assert.Equal(t, storage.CertAuditEventRenewed, event.EventType)
}

func TestCertAuditor_LogRejected(t *testing.T) {
	auditor, store := setupAuditTest(t)
	ctx := context.Background()

	err := auditor.LogRejected(ctx, "agent-4", "ca-1", "invalid token", "agent-4", "192.168.1.100")
	require.NoError(t, err)

	require.Len(t, store.events, 1)
	event := store.events[0]
	assert.Equal(t, "agent-4", event.AgentID)
	assert.Equal(t, storage.CertAuditEventRejected, event.EventType)
	assert.Equal(t, "invalid token", event.Reason)
}

func TestCertAuditor_LogExpired(t *testing.T) {
	auditor, store := setupAuditTest(t)
	ctx := context.Background()

	err := auditor.LogExpired(ctx, "agent-5", "exp123", "ca-1")
	require.NoError(t, err)

	require.Len(t, store.events, 1)
	event := store.events[0]
	assert.Equal(t, "agent-5", event.AgentID)
	assert.Equal(t, "exp123", event.Serial)
	assert.Equal(t, storage.CertAuditEventExpired, event.EventType)
	assert.Equal(t, "system", event.RequestedBy)
}

func TestCertAuditor_GetEvents(t *testing.T) {
	auditor, _ := setupAuditTest(t)
	ctx := context.Background()

	// Log multiple events
	_ = auditor.LogIssued(ctx, "agent-1", "cert1", "ca-1", "user1", "192.168.1.1")
	_ = auditor.LogIssued(ctx, "agent-2", "cert2", "ca-1", "user2", "192.168.1.2")
	_ = auditor.LogRevoked(ctx, "agent-1", "cert1", "ca-1", "expired", "admin")

	// Get all events
	events, err := auditor.GetEvents(ctx, storage.CertAuditFilter{})
	require.NoError(t, err)
	assert.Len(t, events, 3)
}

func TestCertAuditor_GetEventsByAgent(t *testing.T) {
	auditor, _ := setupAuditTest(t)
	ctx := context.Background()

	// Log events for different agents
	_ = auditor.LogIssued(ctx, "agent-1", "cert1", "ca-1", "user1", "192.168.1.1")
	_ = auditor.LogIssued(ctx, "agent-2", "cert2", "ca-1", "user2", "192.168.1.2")
	_ = auditor.LogRenewed(ctx, "agent-1", "cert3", "ca-1", "agent-1", "192.168.1.1")

	// Get events for agent-1 only
	events, err := auditor.GetEventsByAgent(ctx, "agent-1", 10)
	require.NoError(t, err)
	assert.Len(t, events, 2)

	for _, event := range events {
		assert.Equal(t, "agent-1", event.AgentID)
	}
}

func TestCertAuditor_GetRecentEvents(t *testing.T) {
	auditor, _ := setupAuditTest(t)
	ctx := context.Background()

	// Log multiple events
	for i := 0; i < 15; i++ {
		_ = auditor.LogIssued(ctx, "agent-1", "cert"+string(rune('a'+i)), "ca-1", "user", "192.168.1.1")
	}

	// Get only recent 5
	events, err := auditor.GetRecentEvents(ctx, time.Time{}, 5)
	require.NoError(t, err)
	assert.Len(t, events, 5)
}
