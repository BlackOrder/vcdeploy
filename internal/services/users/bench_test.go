package users

import (
	"context"
	"fmt"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
)

func BenchmarkService_Create(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_, err := svc.Create(ctx, fmt.Sprintf("benchuser%d", i), "SecurePass123!", fmt.Sprintf("bench%d@example.com", i), "user")
		if err != nil {
			b.Fatalf("Failed to create user: %v", err)
		}
	}
}

func BenchmarkService_GetByUsername(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	// Setup: create test user
	_, err := svc.Create(ctx, "benchuser", "SecurePass123!", "bench@example.com", "user")
	if err != nil {
		b.Fatalf("Failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := svc.GetByUsername(ctx, "benchuser")
		if err != nil {
			b.Fatalf("Failed to get user: %v", err)
		}
	}
}

func BenchmarkService_GetByID(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	// Setup: create test user
	user, err := svc.Create(ctx, "benchuser", "SecurePass123!", "bench@example.com", "user")
	if err != nil {
		b.Fatalf("Failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := svc.GetByID(ctx, user.ID)
		if err != nil {
			b.Fatalf("Failed to get user: %v", err)
		}
	}
}

func BenchmarkService_List(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	// Setup: create 100 users
	for i := range 100 {
		_, err := svc.Create(ctx, fmt.Sprintf("listuser%d", i), "SecurePass123!", fmt.Sprintf("list%d@example.com", i), "user")
		if err != nil {
			b.Fatalf("Failed to create user: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := svc.List(ctx)
		if err != nil {
			b.Fatalf("Failed to list users: %v", err)
		}
	}
}

func BenchmarkService_Count(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	// Setup: create some users
	for i := range 50 {
		_, err := svc.Create(ctx, fmt.Sprintf("countuser%d", i), "SecurePass123!", fmt.Sprintf("count%d@example.com", i), "user")
		if err != nil {
			b.Fatalf("Failed to create user: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := svc.Count(ctx)
		if err != nil {
			b.Fatalf("Failed to count users: %v", err)
		}
	}
}

func BenchmarkService_VerifyPassword(b *testing.B) {
	db, cleanup := testutil.SetupBenchDB(b)
	defer cleanup()

	svc := New(db)
	ctx := context.Background()

	// Setup: create test user
	_, err := svc.Create(ctx, "authuser", "SecurePass123!", "auth@example.com", "user")
	if err != nil {
		b.Fatalf("Failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := svc.VerifyPassword(ctx, "authuser", "SecurePass123!")
		if err != nil {
			b.Fatalf("Failed to verify password: %v", err)
		}
	}
}
