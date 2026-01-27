package testutil_test

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/testutil"
	fixturesutil "github.com/BlackOrder/vcdeploy/internal/testutil/fixtures"
)

func TestNewTestDB(t *testing.T) {
	t.Parallel()
	testDB := testutil.NewTestDB(t)

	// Verify we can use the database
	if testDB.DB == nil {
		t.Fatal("expected non-nil DB")
	}

	// Verify we can perform operations
	ctx := testutil.TestContext(t)
	user := &storage.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$test",
		Email:        "test@example.com",
		Role:         "admin",
	}

	err := testDB.DB.CreateUser(ctx, user)
	testutil.RequireNoError(t, err, "creating user")

	// Verify we can read back
	got, err := testDB.DB.GetUserByUsername(ctx, "testuser")
	testutil.RequireNoError(t, err, "getting user")

	if got.Email != user.Email {
		t.Errorf("got email %q, want %q", got.Email, user.Email)
	}
}

func TestInMemoryDB(t *testing.T) {
	t.Parallel()
	testDB := testutil.InMemoryDB(t)

	// Verify we can use the database
	if testDB.DB == nil {
		t.Fatal("expected non-nil DB")
	}

	// Verify we can perform operations
	ctx := testutil.TestContext(t)
	user := &storage.User{
		Username:     "memuser",
		PasswordHash: "$2a$10$test",
		Email:        "mem@example.com",
		Role:         "viewer",
	}

	err := testDB.DB.CreateUser(ctx, user)
	testutil.RequireNoError(t, err, "creating user")
}

func TestDBFixtures(t *testing.T) {
	t.Parallel()
	testDB := testutil.NewTestDB(t)
	ctx := testutil.TestContext(t)
	fixtures := fixturesutil.NewDBFixtures(t)

	// Test DefaultAgent
	agent := fixtures.DefaultAgent()
	err := testDB.DB.UpsertAgent(ctx, agent)
	testutil.RequireNoError(t, err, "upserting agent")

	// Verify agent was created
	got, err := testDB.DB.GetAgent(ctx, agent.ID)
	testutil.RequireNoError(t, err, "getting agent")

	if got.Hostname != agent.Hostname {
		t.Errorf("got hostname %q, want %q", got.Hostname, agent.Hostname)
	}

	// Test DefaultDeployment
	deployment := fixtures.DefaultDeployment()
	err = testDB.DB.CreateDeployment(ctx, deployment)
	testutil.RequireNoError(t, err, "creating deployment")
}

func TestSeedTestDB(t *testing.T) {
	t.Parallel()
	testDB := testutil.NewTestDB(t)
	fixtures := fixturesutil.NewDBFixtures(t)
	ctx := testutil.TestContext(t)

	// Seed the database
	err := fixtures.SeedTestDB(testDB.DB)
	testutil.RequireNoError(t, err, "seeding test db")

	// Verify agents were created
	agents, err := testDB.DB.ListAgents(ctx)
	testutil.RequireNoError(t, err, "listing agents")

	if len(agents) != 2 {
		t.Errorf("got %d agents, want 2", len(agents))
	}

	// Verify users were created
	users, err := testDB.DB.ListUsers(ctx)
	testutil.RequireNoError(t, err, "listing users")

	if len(users) != 3 {
		t.Errorf("got %d users, want 3", len(users))
	}
}

func TestTestDBPool(t *testing.T) {
	t.Parallel()
	pool := testutil.NewTestDBPool(t, 5)

	// Get multiple databases
	db1 := pool.Get(t)
	db2 := pool.Get(t)

	// Verify they are different instances
	if db1.Path == db2.Path {
		t.Error("expected different database paths")
	}

	// Verify both work
	ctx := testutil.TestContext(t)
	user := &storage.User{
		Username:     "pooluser",
		PasswordHash: "$2a$10$test",
		Email:        "pool@example.com",
		Role:         "admin",
	}

	err := db1.DB.CreateUser(ctx, user)
	testutil.RequireNoError(t, err, "creating user in db1")

	// User should not exist in db2 (isolation)
	_, err = db2.DB.GetUserByUsername(ctx, "pooluser")
	testutil.RequireError(t, err, "user should not exist in db2")
}
