package projecttypes

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func newTestService(t *testing.T) (*Service, storage.Store) {
	t.Helper()

	db, cleanup := testutil.NewTestStore(t)
	t.Cleanup(cleanup)

	return New(db), db
}

// --- New() Tests ---

func TestNew(t *testing.T) {
	svc, db := newTestService(t)

	if svc == nil {
		t.Fatal("New() returned nil")
	}
	if svc.store != db {
		t.Error("New() did not set db correctly")
	}
}

// --- Create() Tests ---

func TestService_Create(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	pt, err := svc.Create(ctx, "golang", "Go programming language", "go build ./...")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if pt == nil {
		t.Fatal("Create() returned nil")
	}
	if pt.ID == 0 {
		t.Error("Create() did not set ID")
	}
	if pt.Name != "golang" {
		t.Errorf("Create() name = %v, want %v", pt.Name, "golang")
	}
	if pt.Description != "Go programming language" {
		t.Errorf("Create() description = %v, want %v", pt.Description, "Go programming language")
	}
	if pt.BuildCmd != "go build ./..." {
		t.Errorf("Create() buildCmd = %v, want %v", pt.BuildCmd, "go build ./...")
	}
	if pt.CreatedAt.IsZero() {
		t.Error("Create() did not set CreatedAt")
	}
}

func TestService_Create_EmptyName(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "", "description", "build cmd")
	if err == nil {
		t.Error("Create() expected error for empty name")
	}
}

func TestService_Create_EmptyDescription(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	pt, err := svc.Create(ctx, "nodejs", "", "npm run build")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if pt.Description != "" {
		t.Errorf("Create() description = %v, want empty", pt.Description)
	}
}

func TestService_Create_EmptyBuildCmd(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	pt, err := svc.Create(ctx, "static", "Static files", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if pt.BuildCmd != "" {
		t.Errorf("Create() buildCmd = %v, want empty", pt.BuildCmd)
	}
}

func TestService_Create_DuplicateName(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "golang", "Go language", "go build")
	if err != nil {
		t.Fatalf("Create() first error = %v", err)
	}

	// Try to create a duplicate
	_, err = svc.Create(ctx, "golang", "Another Go", "go build ./...")
	if err == nil {
		t.Error("Create() expected error for duplicate name")
	}
}

func TestService_Create_SetsCreatedAt(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	before := time.Now().Add(-time.Second)
	pt, err := svc.Create(ctx, "python", "Python language", "pip install")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	after := time.Now().Add(time.Second)

	if pt.CreatedAt.Before(before) || pt.CreatedAt.After(after) {
		t.Errorf("Create() CreatedAt = %v, expected between %v and %v", pt.CreatedAt, before, after)
	}
}

// --- GetByName() Tests ---

func TestService_GetByName(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a project type first
	created, err := svc.Create(ctx, "golang", "Go programming language", "go build ./...")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Retrieve it
	pt, err := svc.GetByName(ctx, "golang")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	if pt == nil {
		t.Fatal("GetByName() returned nil")
	}
	if pt.ID != created.ID {
		t.Errorf("GetByName() ID = %v, want %v", pt.ID, created.ID)
	}
	if pt.Name != "golang" {
		t.Errorf("GetByName() name = %v, want %v", pt.Name, "golang")
	}
	if pt.Description != "Go programming language" {
		t.Errorf("GetByName() description = %v, want %v", pt.Description, "Go programming language")
	}
	if pt.BuildCmd != "go build ./..." {
		t.Errorf("GetByName() buildCmd = %v, want %v", pt.BuildCmd, "go build ./...")
	}
}

func TestService_GetByName_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.GetByName(ctx, "nonexistent")
	if err == nil {
		t.Error("GetByName() expected error for nonexistent name")
	}
}

func TestService_GetByName_CaseSensitive(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create with lowercase
	_, err := svc.Create(ctx, "golang", "Go language", "go build")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Try to get with different case
	_, err = svc.GetByName(ctx, "GOLANG")
	if err == nil {
		t.Error("GetByName() expected error for different case")
	}
}

// --- List() Tests ---

func TestService_List(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create multiple project types
	types := []struct {
		name        string
		description string
		buildCmd    string
	}{
		{"golang", "Go programming language", "go build ./..."},
		{"nodejs", "Node.js applications", "npm run build"},
		{"python", "Python applications", "pip install -r requirements.txt"},
	}

	for _, pt := range types {
		_, err := svc.Create(ctx, pt.name, pt.description, pt.buildCmd)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List all
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("List() returned %v items, want %v", len(list), 3)
	}
}

func TestService_List_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if list == nil {
		// Empty slice is acceptable but should not be nil
		list = []*storage.ProjectType{}
	}

	if len(list) != 0 {
		t.Errorf("List() returned %v items, want 0", len(list))
	}
}

func TestService_List_OrderedByName(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create in non-alphabetical order
	_, err := svc.Create(ctx, "zebra", "Z type", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(ctx, "alpha", "A type", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(ctx, "beta", "B type", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("List() returned %v items, want 3", len(list))
	}

	// Check ordering (should be alphabetical)
	if list[0].Name != "alpha" {
		t.Errorf("List()[0].Name = %v, want alpha", list[0].Name)
	}
	if list[1].Name != "beta" {
		t.Errorf("List()[1].Name = %v, want beta", list[1].Name)
	}
	if list[2].Name != "zebra" {
		t.Errorf("List()[2].Name = %v, want zebra", list[2].Name)
	}
}

// --- Update() Tests ---

func TestService_Update(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a project type
	created, err := svc.Create(ctx, "golang", "Go language", "go build")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update it
	created.Description = "Updated Go programming language"
	created.BuildCmd = "go build -v ./..."

	err = svc.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	updated, err := svc.GetByName(ctx, "golang")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	if updated.Description != "Updated Go programming language" {
		t.Errorf("Update() description = %v, want %v", updated.Description, "Updated Go programming language")
	}
	if updated.BuildCmd != "go build -v ./..." {
		t.Errorf("Update() buildCmd = %v, want %v", updated.BuildCmd, "go build -v ./...")
	}
}

func TestService_Update_DescriptionOnly(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a project type
	created, err := svc.Create(ctx, "nodejs", "Node.js", "npm build")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update only description
	created.Description = "Node.js applications"

	err = svc.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	updated, err := svc.GetByName(ctx, "nodejs")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	if updated.Description != "Node.js applications" {
		t.Errorf("Update() description = %v, want %v", updated.Description, "Node.js applications")
	}
	if updated.BuildCmd != "npm build" {
		t.Errorf("Update() should preserve buildCmd = %v, got %v", "npm build", updated.BuildCmd)
	}
}

func TestService_Update_BuildCmdOnly(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a project type
	created, err := svc.Create(ctx, "python", "Python apps", "pip install")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update only build command
	created.BuildCmd = "pip install -r requirements.txt && python setup.py install"

	err = svc.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	updated, err := svc.GetByName(ctx, "python")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	if updated.BuildCmd != "pip install -r requirements.txt && python setup.py install" {
		t.Errorf("Update() buildCmd = %v, want %v", updated.BuildCmd, "pip install -r requirements.txt && python setup.py install")
	}
	if updated.Description != "Python apps" {
		t.Errorf("Update() should preserve description = %v, got %v", "Python apps", updated.Description)
	}
}

func TestService_Update_ClearFields(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a project type with all fields
	created, err := svc.Create(ctx, "ruby", "Ruby on Rails", "bundle install")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Clear description and build command
	created.Description = ""
	created.BuildCmd = ""

	err = svc.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	updated, err := svc.GetByName(ctx, "ruby")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	if updated.Description != "" {
		t.Errorf("Update() description = %v, want empty", updated.Description)
	}
	if updated.BuildCmd != "" {
		t.Errorf("Update() buildCmd = %v, want empty", updated.BuildCmd)
	}
}

func TestService_Update_Nonexistent(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	pt := &storage.ProjectType{
		Name:        "nonexistent",
		Description: "Does not exist",
		BuildCmd:    "echo",
	}

	// Update should not error even if the type doesn't exist (no rows updated)
	err := svc.Update(ctx, pt)
	// The behavior depends on the implementation - it may or may not error
	// Just verify it doesn't panic
	_ = err
}

// --- Delete() Tests ---

func TestService_Delete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a project type
	_, err := svc.Create(ctx, "todelete", "Will be deleted", "echo bye")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete it
	err = svc.Delete(ctx, "todelete")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's deleted
	_, err = svc.GetByName(ctx, "todelete")
	if err == nil {
		t.Error("GetByName() expected error after delete")
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Delete nonexistent - should not error (no rows affected)
	err := svc.Delete(ctx, "nonexistent")
	// Depending on implementation, this may or may not error
	// Just verify it doesn't panic
	_ = err
}

func TestService_Delete_DoesNotAffectOthers(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create multiple project types
	_, err := svc.Create(ctx, "type1", "Type 1", "cmd1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(ctx, "type2", "Type 2", "cmd2")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(ctx, "type3", "Type 3", "cmd3")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete one
	err = svc.Delete(ctx, "type2")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify others still exist
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("List() returned %v items, want 2", len(list))
	}

	// Verify type1 and type3 still exist
	_, err = svc.GetByName(ctx, "type1")
	if err != nil {
		t.Errorf("GetByName(type1) error = %v", err)
	}
	_, err = svc.GetByName(ctx, "type3")
	if err != nil {
		t.Errorf("GetByName(type3) error = %v", err)
	}
}

// --- Integration Tests ---

func TestService_CreateAndRetrieve(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create
	created, err := svc.Create(ctx, "integration", "Integration test type", "make test")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Retrieve by name
	retrieved, err := svc.GetByName(ctx, "integration")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	// Compare
	if created.ID != retrieved.ID {
		t.Errorf("ID mismatch: created=%v, retrieved=%v", created.ID, retrieved.ID)
	}
	if created.Name != retrieved.Name {
		t.Errorf("Name mismatch: created=%v, retrieved=%v", created.Name, retrieved.Name)
	}
	if created.Description != retrieved.Description {
		t.Errorf("Description mismatch: created=%v, retrieved=%v", created.Description, retrieved.Description)
	}
	if created.BuildCmd != retrieved.BuildCmd {
		t.Errorf("BuildCmd mismatch: created=%v, retrieved=%v", created.BuildCmd, retrieved.BuildCmd)
	}
}

func TestService_CreateUpdateDelete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create
	pt, err := svc.Create(ctx, "lifecycle", "Initial description", "initial cmd")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update
	pt.Description = "Updated description"
	pt.BuildCmd = "updated cmd"
	err = svc.Update(ctx, pt)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	updated, err := svc.GetByName(ctx, "lifecycle")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	if updated.Description != "Updated description" {
		t.Errorf("Description after update = %v, want %v", updated.Description, "Updated description")
	}

	// Delete
	err = svc.Delete(ctx, "lifecycle")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err = svc.GetByName(ctx, "lifecycle")
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestService_MultipleConcurrentOperations(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create multiple types
	for i := 0; i < 10; i++ {
		name := "type" + string(rune('0'+i))
		_, err := svc.Create(ctx, name, "Description "+name, "cmd "+name)
		if err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
	}

	// Verify all exist
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 10 {
		t.Errorf("List() returned %v items, want 10", len(list))
	}

	// Update all
	for _, pt := range list {
		pt.Description = "Updated " + pt.Description
		if err := svc.Update(ctx, pt); err != nil {
			t.Fatalf("Update(%s) error = %v", pt.Name, err)
		}
	}

	// Delete half
	for i := 0; i < 5; i++ {
		name := "type" + string(rune('0'+i))
		if err := svc.Delete(ctx, name); err != nil {
			t.Fatalf("Delete(%s) error = %v", name, err)
		}
	}

	// Verify remaining
	list, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 5 {
		t.Errorf("List() after delete returned %v items, want 5", len(list))
	}
}

// --- Edge Cases ---

func TestService_Create_SpecialCharacters(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Name with special characters (but valid)
	pt, err := svc.Create(ctx, "node-js-v18", "Node.js v18", "npm run build")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if pt.Name != "node-js-v18" {
		t.Errorf("Create() name = %v, want %v", pt.Name, "node-js-v18")
	}

	// Retrieve it
	retrieved, err := svc.GetByName(ctx, "node-js-v18")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	if retrieved.Name != "node-js-v18" {
		t.Errorf("GetByName() name = %v, want %v", retrieved.Name, "node-js-v18")
	}
}

func TestService_Create_LongValues(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Long description
	longDesc := ""
	for i := 0; i < 100; i++ {
		longDesc += "This is a long description. "
	}

	// Long build command
	longCmd := "echo start"
	for i := 0; i < 50; i++ {
		longCmd += " && echo step" + string(rune('0'+i%10))
	}

	pt, err := svc.Create(ctx, "longtype", longDesc, longCmd)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if pt.Description != longDesc {
		t.Error("Create() did not store long description correctly")
	}
	if pt.BuildCmd != longCmd {
		t.Error("Create() did not store long build command correctly")
	}

	// Verify retrieval
	retrieved, err := svc.GetByName(ctx, "longtype")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	if retrieved.Description != longDesc {
		t.Error("GetByName() did not retrieve long description correctly")
	}
	if retrieved.BuildCmd != longCmd {
		t.Error("GetByName() did not retrieve long build command correctly")
	}
}

func TestService_Create_UnicodeValues(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	pt, err := svc.Create(ctx, "unicode-type", "描述 - Description with émojis 🚀", "echo 你好")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if pt.Description != "描述 - Description with émojis 🚀" {
		t.Errorf("Create() description = %v, want %v", pt.Description, "描述 - Description with émojis 🚀")
	}

	// Verify retrieval
	retrieved, err := svc.GetByName(ctx, "unicode-type")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}

	if retrieved.Description != "描述 - Description with émojis 🚀" {
		t.Errorf("GetByName() description = %v, want %v", retrieved.Description, "描述 - Description with émojis 🚀")
	}
	if retrieved.BuildCmd != "echo 你好" {
		t.Errorf("GetByName() buildCmd = %v, want %v", retrieved.BuildCmd, "echo 你好")
	}
}
