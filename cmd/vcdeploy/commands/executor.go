// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

// ExecutionMode represents the mode of CLI execution.
type ExecutionMode int

const (
	// ModeAPI uses HTTP API (server running).
	ModeAPI ExecutionMode = iota
	// ModeDirect uses direct DB access (offline).
	ModeDirect
)

// String returns a human-readable description of the execution mode.
func (m ExecutionMode) String() string {
	switch m {
	case ModeAPI:
		return "API"
	case ModeDirect:
		return "Direct"
	default:
		return "Unknown"
	}
}

// CLIExecutor provides unified execution for CLI commands.
// It automatically selects API mode (when server is running) or
// direct DB mode (when offline).
type CLIExecutor struct {
	cmd      *cobra.Command
	api      *apiClient   // Set when using API mode
	services *CLIServices // Set when using direct mode
	mode     ExecutionMode
}

// NewExecutor creates a CLI executor with automatic mode detection.
// Priority:
// 1. If --master flag or VCDEPLOY_MASTER env is set → API mode (remote)
// 2. If local server is running → API mode (local, uses Unix socket or localhost)
// 3. Otherwise → Direct mode (offline, requires root/sudo or file permissions)
func NewExecutor(cmd *cobra.Command) (*CLIExecutor, func(), error) {
	exec := &CLIExecutor{cmd: cmd}
	cleanup := func() {}

	// Check for explicit remote mode (--master flag or env var)
	master, _ := cmd.Flags().GetString("master")
	if master == "" {
		master = os.Getenv("VCDEPLOY_MASTER")
	}

	// Check for --offline flag to force direct mode
	offline, _ := cmd.Flags().GetBool("offline")

	// If master specified, use API mode (remote) - requires token
	if master != "" && !offline {
		api, err := newAPIClient(cmd)
		if err != nil {
			return nil, nil, err
		}
		exec.api = api
		exec.mode = ModeAPI
		return exec, cleanup, nil
	}

	// If not forcing offline, check if local server is running
	if !offline && isLocalServerRunning() {
		// Try Unix socket first, then localhost TCP
		api, err := newLocalAPIClient()
		if err != nil {
			// Fall back to TCP localhost with token if socket fails
			tcpAPI, tcpErr := newLocalTCPClient(cmd)
			if tcpErr != nil {
				// Both failed, fall through to direct mode
				goto directMode
			}
			exec.api = tcpAPI
			exec.mode = ModeAPI
			return exec, cleanup, nil
		}
		exec.api = api
		exec.mode = ModeAPI
		return exec, cleanup, nil
	}

directMode:
	// Fall back to direct mode (offline)
	// This requires appropriate permissions (root/sudo or vcdeploy group)
	if err := checkDirectModeAccess(); err != nil {
		return nil, nil, err
	}

	dbPath, err := getDBPath()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot determine database path: %w", err)
	}

	svc, cleanupFn, err := InitCLIServices(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("initialise CLI services: %w", err)
	}
	exec.services = svc
	exec.mode = ModeDirect
	cleanup = cleanupFn

	return exec, cleanup, nil
}

// IsRemote returns true if using API mode (local or remote server).
func (e *CLIExecutor) IsRemote() bool {
	return e.mode == ModeAPI
}

// IsOffline returns true if using direct DB mode.
func (e *CLIExecutor) IsOffline() bool {
	return e.mode == ModeDirect
}

// Mode returns the execution mode.
func (e *CLIExecutor) Mode() ExecutionMode {
	return e.mode
}

// API returns the API client (nil if in direct mode).
func (e *CLIExecutor) API() *apiClient {
	return e.api
}

// Services returns the CLI services (nil if in API mode).
func (e *CLIExecutor) Services() *CLIServices {
	return e.services
}

// ModeString returns a human-readable description of the execution mode.
func (e *CLIExecutor) ModeString() string {
	switch e.mode {
	case ModeAPI:
		if e.api != nil {
			return fmt.Sprintf("API (%s)", e.api.baseURL)
		}
		return "API"
	case ModeDirect:
		return "Direct (offline)"
	default:
		return "Unknown"
	}
}

// checkDirectModeAccess verifies the user has permission for direct DB access.
// Only root/sudo users or members of the vcdeploy group are allowed.
func checkDirectModeAccess() error {
	// Root (UID 0) always has access
	if os.Getuid() == 0 {
		return nil
	}

	// Check if user is in vcdeploy group
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("cannot determine current user: %w", err)
	}

	groups, err := currentUser.GroupIds()
	if err != nil {
		return fmt.Errorf("cannot get user groups: %w", err)
	}

	vcdeployGroup, err := user.LookupGroup("vcdeploy")
	if err != nil {
		// Group doesn't exist - check if DB file is accessible
		dbPath, pathErr := getDBPath()
		if pathErr == nil {
			if _, statErr := os.Stat(dbPath); statErr == nil {
				// DB exists and is accessible, allow access
				return nil
			}
		}
		// Group doesn't exist and DB not accessible
		return fmt.Errorf("permission denied: vcdeploy group not found\n" +
			"Options:\n" +
			"  1. Run with sudo\n" +
			"  2. Use API mode: vcdeploy --master=http://localhost:8080 --token=<token> ...")
	}

	for _, gid := range groups {
		if gid == vcdeployGroup.Gid {
			return nil // User is in vcdeploy group
		}
	}

	// Not root, not in vcdeploy group
	return fmt.Errorf("permission denied: direct mode requires root or vcdeploy group membership\n"+
		"Options:\n"+
		"  1. Run with sudo\n"+
		"  2. Add yourself to vcdeploy group: sudo usermod -aG vcdeploy %s\n"+
		"  3. Use API mode: vcdeploy --master=http://localhost:8080 --token=<token> ...",
		currentUser.Username)
}

// isLocalServerRunning checks if a local vcdeploy server is running.
func isLocalServerRunning() bool {
	// Check Unix socket first
	socketPath := getLocalSocketPath()
	if info, err := os.Stat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket != 0 {
			// Socket exists, try to connect
			conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
			if err == nil {
				conn.Close()
				return true
			}
		}
	}

	// Fall back to TCP check on default ports
	ports := []string{":8080", ":9090"}
	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", "localhost"+port, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}

// newLocalAPIClient creates an API client that connects via Unix socket.
// Unix socket permissions provide authentication - if you can connect, you're authorised.
func newLocalAPIClient() (*apiClient, error) {
	socketPath := getLocalSocketPath()

	// Check socket exists and is accessible
	info, err := os.Stat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("server socket not found at %s", socketPath)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied: cannot access server socket %s", socketPath)
		}
		return nil, err
	}

	// Verify it's a socket
	if info.Mode()&os.ModeSocket == 0 {
		return nil, fmt.Errorf("%s is not a Unix socket", socketPath)
	}

	// Create HTTP client that connects via Unix socket
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}

	return &apiClient{
		baseURL: "http://localhost", // Host header (socket ignores this)
		client:  &http.Client{Transport: transport, Timeout: 30 * time.Second},
		// No token needed - Unix socket permissions are the auth
	}, nil
}

// newLocalTCPClient creates an API client for localhost TCP with token.
func newLocalTCPClient(cmd *cobra.Command) (*apiClient, error) {
	token, _ := cmd.Flags().GetString("token")
	if token == "" {
		token = os.Getenv("VCDEPLOY_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("API token required for local TCP connection (--token or VCDEPLOY_TOKEN)")
	}

	return &apiClient{
		baseURL: "http://localhost:8080",
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// getLocalSocketPath returns the path to the local Unix socket.
func getLocalSocketPath() string {
	// Check environment variable first
	if path := os.Getenv("VCDEPLOY_SOCKET"); path != "" {
		return path
	}
	// Default location
	return "/var/run/vcdeploy/vcdeploy.sock"
}

// getVCDeployGID returns the GID of the vcdeploy group, or -1 if not found.
func getVCDeployGID() int {
	group, err := user.LookupGroup("vcdeploy")
	if err != nil {
		return -1
	}
	gid, _ := strconv.Atoi(group.Gid)
	return gid
}
