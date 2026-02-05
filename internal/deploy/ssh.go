// Package deploy provides SSH-based command execution.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/validation"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHRunner executes commands via SSH.
type SSHRunner struct {
	config     *SSHConfig
	client     *ssh.Client
	jumpClient *ssh.Client // Jump server connection (if using jump host)
	validator  *security.CommandValidator
	mu         sync.Mutex
	lastUsed   time.Time
}

// Ensure SSHRunner implements CommandRunner interface
var _ CommandRunner = (*SSHRunner)(nil)

// SSHConfig contains SSH connection settings.
type SSHConfig struct {
	Host              string
	Port              int
	User              string
	KeyPath           string
	KeyPassphrase     string
	Password          string
	Timeout           time.Duration
	KeepaliveInterval time.Duration

	// Jump server settings
	JumpHost    string
	JumpPort    int
	JumpUser    string
	JumpKeyPath string

	// Host key verification settings
	KnownHostsPath        string // Path to known_hosts file (default: ~/.ssh/known_hosts)
	TrustOnFirstUse       bool   // TOFU mode: automatically add unknown hosts
	StrictHostKey         bool   // If true, reject unknown hosts even in TOFU mode
	InsecureIgnoreHostKey bool   // If true, skip host key verification (for testing only)
}

// NewSSHRunner creates a new SSH runner with default command validation.
func NewSSHRunner(config *SSHConfig) (*SSHRunner, error) {
	return NewSSHRunnerWithValidator(config, security.NewCommandValidator())
}

// NewSSHRunnerWithValidator creates a new SSH runner with a custom validator.
func NewSSHRunnerWithValidator(config *SSHConfig, validator *security.CommandValidator) (*SSHRunner, error) {
	if config.Port == 0 {
		config.Port = 22
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.JumpPort == 0 {
		config.JumpPort = 22
	}

	return &SSHRunner{
		config:    config,
		validator: validator,
	}, nil
}

// Connect establishes the SSH connection.
func (r *SSHRunner) Connect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client != nil {
		return nil // Already connected
	}

	// Validate required connection parameters
	if r.config.Host == "" {
		return errors.New("ssh: host is required")
	}
	if r.config.User == "" {
		return errors.New("ssh: user is required")
	}

	clientConfig, err := r.buildClientConfig()
	if err != nil {
		return fmt.Errorf("building ssh config: %w", err)
	}

	addr := net.JoinHostPort(r.config.Host, strconv.Itoa(r.config.Port))

	// Connect via jump server if configured
	if r.config.JumpHost != "" {
		jumpConfig, err := r.buildJumpConfig()
		if err != nil {
			return fmt.Errorf("building jump config: %w", err)
		}

		jumpAddr := net.JoinHostPort(r.config.JumpHost, strconv.Itoa(r.config.JumpPort))
		jumpClient, err := ssh.Dial("tcp", jumpAddr, jumpConfig)
		if err != nil {
			return fmt.Errorf("connecting to jump server: %w", err)
		}

		// Dial target through jump server
		conn, err := jumpClient.Dial("tcp", addr)
		if err != nil {
			jumpClient.Close()
			return fmt.Errorf("connecting through jump server: %w", err)
		}

		ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
		if err != nil {
			conn.Close()
			jumpClient.Close()
			return fmt.Errorf("ssh handshake through jump: %w", err)
		}

		r.client = ssh.NewClient(ncc, chans, reqs)
		r.jumpClient = jumpClient // Store for cleanup
	} else {
		// Direct connection
		dialer := net.Dialer{Timeout: r.config.Timeout}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("dialing %s: %w", addr, err)
		}

		ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
		if err != nil {
			conn.Close()
			return fmt.Errorf("ssh handshake: %w", err)
		}

		r.client = ssh.NewClient(ncc, chans, reqs)
	}

	r.lastUsed = time.Now()
	return nil
}

func (r *SSHRunner) buildClientConfig() (*ssh.ClientConfig, error) {
	var authMethods []ssh.AuthMethod

	// Try key authentication first
	if r.config.KeyPath != "" {
		signer, err := r.loadKey(r.config.KeyPath, r.config.KeyPassphrase)
		if err != nil {
			return nil, fmt.Errorf("loading key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	// Fallback to password
	if r.config.Password != "" {
		authMethods = append(authMethods, ssh.Password(r.config.Password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods configured")
	}

	// Build host key callback with proper verification
	hostKeyCallback, err := r.buildHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("building host key callback: %w", err)
	}

	return &ssh.ClientConfig{
		User:            r.config.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         r.config.Timeout,
	}, nil
}

func (r *SSHRunner) buildJumpConfig() (*ssh.ClientConfig, error) {
	user := r.config.JumpUser
	if user == "" {
		user = r.config.User
	}

	keyPath := r.config.JumpKeyPath
	if keyPath == "" {
		keyPath = r.config.KeyPath
	}

	var authMethods []ssh.AuthMethod
	if keyPath != "" {
		signer, err := r.loadKey(keyPath, "")
		if err != nil {
			return nil, fmt.Errorf("loading jump key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	// Build host key callback with proper verification
	hostKeyCallback, err := r.buildHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("building host key callback: %w", err)
	}

	return &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         r.config.Timeout,
	}, nil
}

// buildHostKeyCallback creates a host key callback with proper verification.
// It supports:
// 1. Standard known_hosts file verification
// 2. TOFU (Trust On First Use) mode for new hosts
// 3. Strict mode that rejects all unknown hosts
func (r *SSHRunner) buildHostKeyCallback() (ssh.HostKeyCallback, error) {
	// For testing only: skip host key verification entirely
	if r.config.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil // #nosec G106 -- user explicitly enabled insecure mode
	}

	// Determine known_hosts file path
	knownHostsPath := r.config.KnownHostsPath
	if knownHostsPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// Fallback to system config path if home dir unavailable
			sysCfg, cfgErr := config.GetSystemConfig()
			if cfgErr != nil {
				return nil, fmt.Errorf("failed to get system config for known_hosts path: %w", cfgErr)
			}
			knownHostsPath = filepath.Join(sysCfg.Paths.DataDir, "known_hosts")
		} else {
			knownHostsPath = filepath.Join(homeDir, ".ssh", "known_hosts")
		}
	}

	// Ensure the directory exists
	knownHostsDir := filepath.Dir(knownHostsPath)
	if err := os.MkdirAll(knownHostsDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating known_hosts directory: %w", err)
	}

	// Create known_hosts file if it doesn't exist
	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		f, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_RDONLY, 0o600) // #nosec G304 - knownHostsPath is admin-controlled SSH config path
		if err != nil {
			return nil, fmt.Errorf("creating known_hosts file: %w", err)
		}
		f.Close()
	}

	// Try to create callback from known_hosts file
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("parsing known_hosts: %w", err)
	}

	// If strict mode, use the callback directly (reject unknown hosts)
	if r.config.StrictHostKey {
		return hostKeyCallback, nil
	}

	// TOFU mode: wrap the callback to add unknown hosts
	if r.config.TrustOnFirstUse {
		return r.tofuCallback(hostKeyCallback, knownHostsPath), nil
	}

	// Default: use standard known_hosts verification
	return hostKeyCallback, nil
}

// tofuCallback wraps a host key callback with Trust On First Use semantics.
// If a host is unknown, it adds the key to known_hosts and allows the connection.
// If a host is known but the key doesn't match, it rejects the connection.
func (r *SSHRunner) tofuCallback(wrapped ssh.HostKeyCallback, knownHostsPath string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := wrapped(hostname, remote, key)
		if err == nil {
			// Host key verified successfully
			return nil
		}

		// Check if this is a "key not found" error vs a "key mismatch" error
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) > 0 {
				// Host is known but key doesn't match - MITM attack or key rotation
				return fmt.Errorf("host key mismatch for %s: expected %s, got %s (possible MITM attack)",
					hostname,
					keyErr.Want[0].Key.Type(),
					key.Type())
			}

			// Host is unknown - add to known_hosts (TOFU)
			if err := r.addToKnownHosts(knownHostsPath, hostname, key); err != nil {
				return fmt.Errorf("failed to add host to known_hosts: %w", err)
			}
			return nil
		}

		// Other error
		return err
	}
}

// addToKnownHosts appends a host key to the known_hosts file.
func (r *SSHRunner) addToKnownHosts(knownHostsPath, hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 - knownHostsPath is admin-controlled SSH config path
	if err != nil {
		return fmt.Errorf("opening known_hosts: %w", err)
	}
	defer f.Close()

	// Format: hostname key-type base64-key
	line := knownhosts.Line([]string{hostname}, key)
	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

func (r *SSHRunner) loadKey(path, passphrase string) (ssh.Signer, error) {
	key, err := os.ReadFile(path) // #nosec G304 - path is admin-controlled SSH key location
	if err != nil {
		return nil, fmt.Errorf("reading key file: %w", err)
	}

	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(key)
}

// Run executes a command and returns the result.
func (r *SSHRunner) Run(ctx context.Context, cmd string, opts RunOptions) (*CommandResult, error) {
	// Validate command if validator present
	if r.validator != nil {
		if err := r.validator.Validate(cmd); err != nil {
			return nil, fmt.Errorf("command validation failed: %w", err)
		}
	}

	if err := r.Connect(ctx); err != nil {
		return nil, err
	}

	r.mu.Lock()
	client := r.client
	r.mu.Unlock()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	// Build command with options
	fullCmd, err := r.buildCommand(cmd, opts)
	if err != nil {
		return nil, fmt.Errorf("building command: %w", err)
	}

	start := time.Now()
	output, err := session.CombinedOutput(fullCmd)
	duration := time.Since(start)

	result := &CommandResult{
		Stdout:   string(output),
		Duration: duration,
	}

	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			return nil, fmt.Errorf("running command: %w", err)
		}
	}

	r.mu.Lock()
	r.lastUsed = time.Now()
	r.mu.Unlock()
	return result, nil
}

// RunWithOutput executes a command with streaming output.
func (r *SSHRunner) RunWithOutput(ctx context.Context, cmd string, stdout, stderr io.Writer, opts RunOptions) error {
	// Validate command if validator present
	if r.validator != nil {
		if err := r.validator.Validate(cmd); err != nil {
			return fmt.Errorf("command validation failed: %w", err)
		}
	}

	if err := r.Connect(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	client := r.client
	r.mu.Unlock()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	fullCmd, err := r.buildCommand(cmd, opts)
	if err != nil {
		return fmt.Errorf("building command: %w", err)
	}

	err = session.Run(fullCmd)
	r.mu.Lock()
	r.lastUsed = time.Now()
	r.mu.Unlock()

	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return err // Return exit error for caller to handle
		}
		return fmt.Errorf("running command: %w", err)
	}

	return nil
}

func (r *SSHRunner) buildCommand(cmd string, opts RunOptions) (string, error) {
	var prefix string

	// Change directory if specified
	if opts.WorkDir != "" {
		prefix += fmt.Sprintf("cd %s && ", opts.WorkDir)
	}

	// Set environment variables
	for k, v := range opts.Env {
		prefix += fmt.Sprintf("export %s=%q && ", k, v)
	}

	// Run as different user if specified
	if opts.User != "" {
		// Validate username to prevent command injection
		if !validation.IsValidUnixUsername(opts.User) {
			return "", fmt.Errorf("invalid username: %q", opts.User)
		}
		// Use sudo to switch user
		return fmt.Sprintf("sudo -u %s bash -c %q", opts.User, prefix+cmd), nil
	}

	return prefix + cmd, nil
}

// Close closes the SSH connection.
func (r *SSHRunner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	if r.client != nil {
		if err := r.client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing client: %w", err))
		}
		r.client = nil
	}
	if r.jumpClient != nil {
		if err := r.jumpClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing jump client: %w", err))
		}
		r.jumpClient = nil
	}
	return errors.Join(errs...)
}

// LastUsed returns when the connection was last used.
func (r *SSHRunner) LastUsed() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastUsed
}

// --- SSH Connection Pool ---

// SSHPool manages a pool of SSH connections.
type SSHPool struct {
	connections map[string]*SSHRunner
	mu          sync.RWMutex
	idleTimeout time.Duration
	stopCh      chan struct{}
}

// NewSSHPool creates a new SSH connection pool.
func NewSSHPool(idleTimeout time.Duration) *SSHPool {
	pool := &SSHPool{
		connections: make(map[string]*SSHRunner),
		idleTimeout: idleTimeout,
		stopCh:      make(chan struct{}),
	}

	// Start cleanup goroutine
	go pool.cleanupLoop()

	return pool
}

// Get returns an SSH runner for the given config, reusing existing connections.
func (p *SSHPool) Get(config *SSHConfig) (*SSHRunner, error) {
	key := fmt.Sprintf("%s@%s:%d", config.User, config.Host, config.Port)
	if config.JumpHost != "" {
		key += fmt.Sprintf("->%s:%d", config.JumpHost, config.JumpPort)
	}

	p.mu.RLock()
	runner, exists := p.connections[key]
	p.mu.RUnlock()

	if exists {
		return runner, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if runner, exists = p.connections[key]; exists {
		return runner, nil
	}

	runner, err := NewSSHRunner(config)
	if err != nil {
		return nil, err
	}

	p.connections[key] = runner
	return runner, nil
}

func (p *SSHPool) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.cleanup()
		}
	}
}

func (p *SSHPool) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	cutoff := time.Now().Add(-p.idleTimeout)
	for key, runner := range p.connections {
		if runner.LastUsed().Before(cutoff) {
			runner.Close()
			delete(p.connections, key)
		}
	}
}

// Close closes all connections in the pool and stops the cleanup goroutine.
func (p *SSHPool) Close() error {
	// Signal cleanup goroutine to stop
	close(p.stopCh)

	p.mu.Lock()
	defer p.mu.Unlock()

	for key, runner := range p.connections {
		runner.Close()
		delete(p.connections, key)
	}
	return nil
}
