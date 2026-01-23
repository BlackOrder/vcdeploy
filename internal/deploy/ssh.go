// Package deploy provides SSH-based command execution.
package deploy

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHRunner executes commands via SSH.
type SSHRunner struct {
	config   *SSHConfig
	client   *ssh.Client
	mu       sync.Mutex
	lastUsed time.Time
}

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
}

// NewSSHRunner creates a new SSH runner.
func NewSSHRunner(config *SSHConfig) (*SSHRunner, error) {
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
		config: config,
	}, nil
}

// Connect establishes the SSH connection.
func (r *SSHRunner) Connect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client != nil {
		return nil // Already connected
	}

	clientConfig, err := r.buildClientConfig()
	if err != nil {
		return fmt.Errorf("building ssh config: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", r.config.Host, r.config.Port)

	// Connect via jump server if configured
	if r.config.JumpHost != "" {
		jumpConfig, err := r.buildJumpConfig()
		if err != nil {
			return fmt.Errorf("building jump config: %w", err)
		}

		jumpAddr := fmt.Sprintf("%s:%d", r.config.JumpHost, r.config.JumpPort)
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

	return &ssh.ClientConfig{
		User:            r.config.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification
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

	return &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         r.config.Timeout,
	}, nil
}

func (r *SSHRunner) loadKey(path, passphrase string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
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
	fullCmd := r.buildCommand(cmd, opts)

	start := time.Now()
	output, err := session.CombinedOutput(fullCmd)
	duration := time.Since(start)

	result := &CommandResult{
		Stdout:   string(output),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			return nil, fmt.Errorf("running command: %w", err)
		}
	}

	r.lastUsed = time.Now()
	return result, nil
}

// RunWithOutput executes a command with streaming output.
func (r *SSHRunner) RunWithOutput(ctx context.Context, cmd string, stdout, stderr io.Writer, opts RunOptions) error {
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

	fullCmd := r.buildCommand(cmd, opts)

	err = session.Run(fullCmd)
	r.lastUsed = time.Now()

	if err != nil {
		if _, ok := err.(*ssh.ExitError); ok {
			return err // Return exit error for caller to handle
		}
		return fmt.Errorf("running command: %w", err)
	}

	return nil
}

func (r *SSHRunner) buildCommand(cmd string, opts RunOptions) string {
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
		// Use sudo to switch user
		return fmt.Sprintf("sudo -u %s bash -c %q", opts.User, prefix+cmd)
	}

	return prefix + cmd
}

// Close closes the SSH connection.
func (r *SSHRunner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client != nil {
		err := r.client.Close()
		r.client = nil
		return err
	}
	return nil
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
}

// NewSSHPool creates a new SSH connection pool.
func NewSSHPool(idleTimeout time.Duration) *SSHPool {
	pool := &SSHPool{
		connections: make(map[string]*SSHRunner),
		idleTimeout: idleTimeout,
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

	for range ticker.C {
		p.cleanup()
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

// Close closes all connections in the pool.
func (p *SSHPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, runner := range p.connections {
		runner.Close()
		delete(p.connections, key)
	}
	return nil
}
