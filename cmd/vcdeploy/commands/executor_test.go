package commands

import (
	"os"
	"os/user"
	"testing"
)

func TestExecutionMode_String(t *testing.T) {
	tests := []struct {
		mode ExecutionMode
		want string
	}{
		{ModeAPI, "API"},
		{ModeDirect, "Direct"},
		{ExecutionMode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("ExecutionMode.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCLIExecutor_ModeString(t *testing.T) {
	t.Run("API mode with URL", func(t *testing.T) {
		exec := &CLIExecutor{
			mode: ModeAPI,
			api:  &apiClient{baseURL: "http://localhost:8080"},
		}
		got := exec.ModeString()
		if got != "API (http://localhost:8080)" {
			t.Errorf("ModeString() = %v, want API (http://localhost:8080)", got)
		}
	})

	t.Run("API mode without client", func(t *testing.T) {
		exec := &CLIExecutor{
			mode: ModeAPI,
			api:  nil,
		}
		got := exec.ModeString()
		if got != "API" {
			t.Errorf("ModeString() = %v, want API", got)
		}
	})

	t.Run("Direct mode", func(t *testing.T) {
		exec := &CLIExecutor{
			mode: ModeDirect,
		}
		got := exec.ModeString()
		if got != "Direct (offline)" {
			t.Errorf("ModeString() = %v, want Direct (offline)", got)
		}
	})
}

func TestCLIExecutor_IsRemote(t *testing.T) {
	t.Run("API mode is remote", func(t *testing.T) {
		exec := &CLIExecutor{mode: ModeAPI}
		if !exec.IsRemote() {
			t.Error("IsRemote() = false, want true for ModeAPI")
		}
	})

	t.Run("Direct mode is not remote", func(t *testing.T) {
		exec := &CLIExecutor{mode: ModeDirect}
		if exec.IsRemote() {
			t.Error("IsRemote() = true, want false for ModeDirect")
		}
	})
}

func TestCLIExecutor_IsOffline(t *testing.T) {
	t.Run("Direct mode is offline", func(t *testing.T) {
		exec := &CLIExecutor{mode: ModeDirect}
		if !exec.IsOffline() {
			t.Error("IsOffline() = false, want true for ModeDirect")
		}
	})

	t.Run("API mode is not offline", func(t *testing.T) {
		exec := &CLIExecutor{mode: ModeAPI}
		if exec.IsOffline() {
			t.Error("IsOffline() = true, want false for ModeAPI")
		}
	})
}

func TestCheckDirectModeAccess_Root(t *testing.T) {
	// Skip if not running as root
	if os.Getuid() != 0 {
		t.Skip("Test requires root privileges")
	}

	err := checkDirectModeAccess()
	if err != nil {
		t.Errorf("checkDirectModeAccess() error = %v, want nil for root", err)
	}
}

func TestCheckDirectModeAccess_NonRoot(t *testing.T) {
	// Skip if running as root
	if os.Getuid() == 0 {
		t.Skip("Test requires non-root user")
	}

	// Check if vcdeploy group exists
	_, groupErr := user.LookupGroup("vcdeploy")
	hasGroup := groupErr == nil

	err := checkDirectModeAccess()

	if hasGroup {
		// If group exists, result depends on group membership
		// Can't easily test without modifying system groups
		t.Log("vcdeploy group exists, skipping detailed check")
	} else {
		// If group doesn't exist, should check DB accessibility
		// Behavior depends on whether DB file is accessible
		t.Log("vcdeploy group does not exist")
		if err != nil {
			// Expected if DB not accessible and not in group
			t.Logf("Got expected error: %v", err)
		}
	}
}

func TestGetLocalSocketPath(t *testing.T) {
	t.Run("default path", func(t *testing.T) {
		// Ensure env var is not set
		os.Unsetenv("VCDEPLOY_SOCKET")

		path := getLocalSocketPath()
		if path != "/var/run/vcdeploy/vcdeploy.sock" {
			t.Errorf("getLocalSocketPath() = %v, want /var/run/vcdeploy/vcdeploy.sock", path)
		}
	})

	t.Run("custom path from env", func(t *testing.T) {
		customPath := "/tmp/test-vcdeploy.sock"
		os.Setenv("VCDEPLOY_SOCKET", customPath)
		defer os.Unsetenv("VCDEPLOY_SOCKET")

		path := getLocalSocketPath()
		if path != customPath {
			t.Errorf("getLocalSocketPath() = %v, want %v", path, customPath)
		}
	})
}

func TestGetVCDeployGID(t *testing.T) {
	gid := getVCDeployGID()

	// Check if vcdeploy group exists
	group, err := user.LookupGroup("vcdeploy")
	if err != nil {
		// Group doesn't exist, should return -1
		if gid != -1 {
			t.Errorf("getVCDeployGID() = %v, want -1 when group doesn't exist", gid)
		}
	} else {
		// Group exists, should return its GID
		if gid == -1 {
			t.Errorf("getVCDeployGID() = -1, want actual GID when group exists")
		}
		t.Logf("vcdeploy group GID: %s (got: %d)", group.Gid, gid)
	}
}

func TestIsLocalServerRunning(t *testing.T) {
	// This test just verifies the function doesn't panic
	// Actual behavior depends on system state
	running := isLocalServerRunning()
	t.Logf("isLocalServerRunning() = %v", running)
}
