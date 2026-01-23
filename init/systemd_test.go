//go:build systemd

package init

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSystemdUnitsExist verifies that the systemd unit files exist.
func TestSystemdUnitsExist(t *testing.T) {
	t.Parallel()

	units := []string{
		"vcdeploy-master.service",
		"vcdeploy-agent.service",
	}

	for _, unit := range units {
		path := findUnitFile(t, unit)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Unit file %s not found", unit)
		}
	}
}

// TestSystemdUnitsValidSyntax verifies the unit files have valid systemd syntax.
// This test requires systemd-analyze to be installed on the system.
func TestSystemdUnitsValidSyntax(t *testing.T) {
	t.Parallel()

	// Check if systemd-analyze is available
	_, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze not found, skipping systemd validation")
	}

	units := []string{
		"vcdeploy-master.service",
		"vcdeploy-agent.service",
	}

	for _, unit := range units {
		t.Run(unit, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("init", unit)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				path = filepath.Join("..", "..", "init", unit)
			}

			cmd := exec.Command("systemd-analyze", "verify", path)
			output, err := cmd.CombinedOutput()

			// systemd-analyze verify returns non-zero even for warnings
			// We check for actual errors in the output
			outputStr := string(output)
			if err != nil && strings.Contains(outputStr, "error:") {
				t.Errorf("systemd-analyze verify %s failed: %s", unit, output)
			}
		})
	}
}

// TestSystemdMasterUnitContents validates the master service unit.
func TestSystemdMasterUnitContents(t *testing.T) {
	t.Parallel()

	path := findUnitFile(t, "vcdeploy-master.service")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read unit file: %v", err)
	}

	text := string(content)

	// Required sections
	if !strings.Contains(text, "[Unit]") {
		t.Error("Missing [Unit] section")
	}
	if !strings.Contains(text, "[Service]") {
		t.Error("Missing [Service] section")
	}
	if !strings.Contains(text, "[Install]") {
		t.Error("Missing [Install] section")
	}

	// Required directives
	if !strings.Contains(text, "Description=") {
		t.Error("Missing Description directive")
	}
	if !strings.Contains(text, "ExecStart=") {
		t.Error("Missing ExecStart directive")
	}
	if !strings.Contains(text, "Restart=") {
		t.Error("Missing Restart directive")
	}

	// Security hardening
	if !strings.Contains(text, "NoNewPrivileges=yes") {
		t.Error("Missing NoNewPrivileges security directive")
	}
	if !strings.Contains(text, "ProtectSystem=") {
		t.Error("Missing ProtectSystem security directive")
	}

	// Installation target
	if !strings.Contains(text, "WantedBy=multi-user.target") {
		t.Error("Should be wanted by multi-user.target")
	}
}

// TestSystemdAgentUnitContents validates the agent service unit.
func TestSystemdAgentUnitContents(t *testing.T) {
	t.Parallel()

	path := findUnitFile(t, "vcdeploy-agent.service")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read unit file: %v", err)
	}

	text := string(content)

	// Required sections
	if !strings.Contains(text, "[Unit]") {
		t.Error("Missing [Unit] section")
	}
	if !strings.Contains(text, "[Service]") {
		t.Error("Missing [Service] section")
	}
	if !strings.Contains(text, "[Install]") {
		t.Error("Missing [Install] section")
	}

	// Required directives
	if !strings.Contains(text, "Description=") {
		t.Error("Missing Description directive")
	}
	if !strings.Contains(text, "ExecStart=") {
		t.Error("Missing ExecStart directive")
	}
	if !strings.Contains(text, "Restart=") {
		t.Error("Missing Restart directive")
	}

	// Agent runs as root for deployment capabilities
	if !strings.Contains(text, "User=root") {
		t.Error("Agent should run as root")
	}

	// Graceful shutdown support
	if !strings.Contains(text, "TimeoutStopSec=") {
		t.Error("Missing TimeoutStopSec for graceful shutdown")
	}

	// Installation target
	if !strings.Contains(text, "WantedBy=multi-user.target") {
		t.Error("Should be wanted by multi-user.target")
	}
}

// TestSystemdUnitNetworkDependency checks After=network.target.
func TestSystemdUnitNetworkDependency(t *testing.T) {
	t.Parallel()

	units := []string{"vcdeploy-master.service", "vcdeploy-agent.service"}

	for _, unit := range units {
		t.Run(unit, func(t *testing.T) {
			t.Parallel()

			path := findUnitFile(t, unit)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read unit file: %v", err)
			}

			if !strings.Contains(string(content), "After=network.target") {
				t.Error("Should have After=network.target for network dependency")
			}
		})
	}
}

// TestSystemdUnitRestartPolicy checks restart configuration.
func TestSystemdUnitRestartPolicy(t *testing.T) {
	t.Parallel()

	units := []string{"vcdeploy-master.service", "vcdeploy-agent.service"}

	for _, unit := range units {
		t.Run(unit, func(t *testing.T) {
			t.Parallel()

			path := findUnitFile(t, unit)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read unit file: %v", err)
			}

			text := string(content)

			if !strings.Contains(text, "Restart=on-failure") {
				t.Error("Should have Restart=on-failure")
			}
			if !strings.Contains(text, "RestartSec=") {
				t.Error("Should have RestartSec for restart delay")
			}
		})
	}
}

// TestSystemdUnitJournalLogging checks journal logging configuration.
func TestSystemdUnitJournalLogging(t *testing.T) {
	t.Parallel()

	units := []string{"vcdeploy-master.service", "vcdeploy-agent.service"}

	for _, unit := range units {
		t.Run(unit, func(t *testing.T) {
			t.Parallel()

			path := findUnitFile(t, unit)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read unit file: %v", err)
			}

			text := string(content)

			if !strings.Contains(text, "StandardOutput=journal") {
				t.Error("Should log stdout to journal")
			}
			if !strings.Contains(text, "StandardError=journal") {
				t.Error("Should log stderr to journal")
			}
			if !strings.Contains(text, "SyslogIdentifier=") {
				t.Error("Should have SyslogIdentifier for filtering logs")
			}
		})
	}
}

// findUnitFile locates the unit file from different working directories.
func findUnitFile(t *testing.T, name string) string {
	t.Helper()

	paths := []string{
		filepath.Join("init", name),
		filepath.Join("..", "init", name),
		filepath.Join("..", "..", "init", name),
		filepath.Join("/opt/code/vcdeploy/init", name),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	t.Fatalf("Unit file %s not found in any location", name)
	return ""
}
