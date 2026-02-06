package export

import (
	"testing"
)

func TestFormatVersion(t *testing.T) {
	if FormatVersion == "" {
		t.Error("FormatVersion should not be empty")
	}
	if FormatVersion[0] != 'v' {
		t.Error("FormatVersion should start with 'v'")
	}
}

func TestExportBundle_Structure(t *testing.T) {
	// Test that ExportBundle can be created with zero values
	bundle := &ExportBundle{}
	if bundle.FormatVersion != "" {
		t.Error("Default FormatVersion should be empty")
	}
	if bundle.Components != nil {
		t.Error("Default Components should be nil")
	}
	if bundle.Playbooks != nil {
		t.Error("Default Playbooks should be nil")
	}
}

func TestComponentExport_Structure(t *testing.T) {
	ce := ComponentExport{
		Slug:          "test-comp",
		Version:       "v1.0.0",
		Name:          "Test",
		ComponentType: "command",
		IsRaw:         false,
	}

	if ce.Slug != "test-comp" {
		t.Errorf("Slug = %v, want test-comp", ce.Slug)
	}
	if ce.Version != "v1.0.0" {
		t.Errorf("Version = %v, want v1.0.0", ce.Version)
	}
	if ce.Name != "Test" {
		t.Errorf("Name = %v, want Test", ce.Name)
	}
	if ce.ComponentType != "command" {
		t.Errorf("ComponentType = %v, want command", ce.ComponentType)
	}
	if ce.IsRaw != false {
		t.Error("Default IsRaw should be false")
	}
}

func TestPlaybookExport_Structure(t *testing.T) {
	pe := PlaybookExport{
		Slug:         "test-playbook",
		Version:      "v1.0.0",
		Name:         "Test Playbook",
		KeepReleases: 5,
	}

	if pe.Slug != "test-playbook" {
		t.Errorf("Slug = %v, want test-playbook", pe.Slug)
	}
	if pe.Version != "v1.0.0" {
		t.Errorf("Version = %v, want v1.0.0", pe.Version)
	}
	if pe.Name != "Test Playbook" {
		t.Errorf("Name = %v, want Test Playbook", pe.Name)
	}
	if pe.KeepReleases != 5 {
		t.Errorf("KeepReleases = %d, want 5", pe.KeepReleases)
	}
}
