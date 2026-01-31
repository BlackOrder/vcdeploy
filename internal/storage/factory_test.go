package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewCachedStore(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "vcdeploy-factory-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := zap.NewNop()

	// Create cached store
	store, err := NewCachedStore(dbPath, logger)
	if err != nil {
		t.Fatalf("NewCachedStore failed: %v", err)
	}
	defer store.Close()

	// Verify we can access the underlying DB
	if store.UnderlyingDB() == nil {
		t.Error("UnderlyingDB() returned nil")
	}

	// Verify we can perform operations through the MemoryStore
	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "testuser",
		PasswordHash: "hash123",
		Email:        "test@example.com",
		Role:         "admin",
	}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Verify we can read it back (from memory)
	readUser, err := store.GetUserByUsername(ctx, "testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if readUser.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", readUser.Email)
	}
}

func TestNewCachedStoreLoadsExistingData(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "vcdeploy-factory-load-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := zap.NewNop()

	// First, create data using direct DB access
	db, err := New(dbPath, logger)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	user := &User{
		Username:     "preexisting",
		PasswordHash: "hash456",
		Email:        "pre@example.com",
		Role:         "viewer",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	db.Close()

	// Now open with CachedStore and verify data is loaded
	store, err := NewCachedStore(dbPath, logger)
	if err != nil {
		t.Fatalf("NewCachedStore failed: %v", err)
	}
	defer store.Close()

	// Data should be in memory now
	readUser, err := store.GetUserByUsername(ctx, "preexisting")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if readUser.Email != "pre@example.com" {
		t.Errorf("Expected email pre@example.com, got %s", readUser.Email)
	}
}

func TestNewCachedStoreWriteThrough(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "vcdeploy-factory-writethrough-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := zap.NewNop()

	// Create cached store
	store, err := NewCachedStore(dbPath, logger)
	if err != nil {
		t.Fatalf("NewCachedStore failed: %v", err)
	}

	ctx := context.Background()

	// Create data through cached store
	user := &User{
		Username:     "writethrough",
		PasswordHash: "hash789",
		Email:        "wt@example.com",
		Role:         "admin",
	}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Close the store (this should flush pending writes)
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Give background workers time to flush
	time.Sleep(200 * time.Millisecond)

	// Open directly with DB to verify data was persisted
	db, err := New(dbPath, logger)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer db.Close()

	readUser, err := db.GetUserByUsername(ctx, "writethrough")
	if err != nil {
		t.Fatalf("GetUserByUsername from DB failed: %v", err)
	}
	if readUser.Email != "wt@example.com" {
		t.Errorf("Expected email wt@example.com, got %s", readUser.Email)
	}
}

func TestNewCachedStoreInvalidPath(t *testing.T) {
	// Try to create store with invalid path
	_, err := NewCachedStore("/nonexistent/directory/test.db", nil)
	if err == nil {
		t.Error("Expected error for invalid path, got nil")
	}
}
