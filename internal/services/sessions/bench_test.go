package sessions

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
)

func BenchmarkService_Create(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := svc.Create(ctx, "test-user-id", "127.0.0.1", "Benchmark Agent", 24*time.Hour)
		if err != nil {
			b.Fatalf("Failed to create session: %v", err)
		}
	}
}

func BenchmarkService_GetByToken(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	// Setup: create test session
	session, err := svc.Create(ctx, "test-user-id", "127.0.0.1", "Benchmark Agent", 24*time.Hour)
	if err != nil {
		b.Fatalf("Failed to create session: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := svc.GetByToken(ctx, session.Token)
		if err != nil {
			b.Fatalf("Failed to get session: %v", err)
		}
	}
}

func BenchmarkService_ListForUser(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	// Setup: create multiple sessions for user
	for range 10 {
		_, err := svc.Create(ctx, "test-user-id", "127.0.0.1", "Benchmark Agent", 24*time.Hour)
		if err != nil {
			b.Fatalf("Failed to create session: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := svc.ListForUser(ctx, "test-user-id")
		if err != nil {
			b.Fatalf("Failed to list sessions: %v", err)
		}
	}
}

func BenchmarkService_Delete(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	// Pre-create sessions for deletion
	tokens := make([]string, b.N)
	for i := range b.N {
		session, err := svc.Create(ctx, "test-user-id", "127.0.0.1", "Benchmark Agent", 24*time.Hour)
		if err != nil {
			b.Fatalf("Failed to create session: %v", err)
		}
		tokens[i] = session.Token
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		err := svc.Delete(ctx, tokens[i])
		if err != nil {
			b.Fatalf("Failed to delete session: %v", err)
		}
	}
}

func BenchmarkService_DeleteExpired(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	// Setup: create some expired sessions (we'll just run the cleanup)
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := svc.DeleteExpired(ctx)
		if err != nil {
			b.Fatalf("Failed to delete expired sessions: %v", err)
		}
	}
}
