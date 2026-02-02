// Package provision handles agent provisioning and lifecycle management.
package provision

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

var (
	// ErrAgentAlreadyInstalled indicates the agent is already installed on the target.
	ErrAgentAlreadyInstalled = errors.New("agent already installed on target")

	// ErrSSHConnectionFailed indicates SSH connection failed.
	ErrSSHConnectionFailed = errors.New("SSH connection failed")

	// ErrInstallationFailed indicates agent installation failed.
	ErrInstallationFailed = errors.New("agent installation failed")

	// ErrSSHKeyNotFound indicates the requested SSH key was not found.
	ErrSSHKeyNotFound = errors.New("SSH key not found")
)

// SSHProvisioner handles remote agent installation via SSH.
type SSHProvisioner struct {
	ssh         *security.SSHKeyManager
	provisioner *Provisioner
	masterURL   string
	binaryURL   string // URL to download agent binary from
	logger      *zap.Logger
	connTimeout time.Duration
	execTimeout time.Duration
}

// SSHProvisionerConfig contains configuration for the SSH provisioner.
type SSHProvisionerConfig struct {
	MasterURL         string
	BinaryURL         string // URL template with {os} and {arch} placeholders
	ConnectionTimeout time.Duration
	ExecutionTimeout  time.Duration
}

// NewSSHProvisioner creates a new SSH provisioner.
func NewSSHProvisioner(ssh *security.SSHKeyManager, provisioner *Provisioner, logger *zap.Logger, cfg SSHProvisionerConfig) *SSHProvisioner {
	connTimeout := cfg.ConnectionTimeout
	if connTimeout == 0 {
		connTimeout = 30 * time.Second
	}

	execTimeout := cfg.ExecutionTimeout
	if execTimeout == 0 {
		execTimeout = 5 * time.Minute
	}

	return &SSHProvisioner{
		ssh:         ssh,
		provisioner: provisioner,
		masterURL:   cfg.MasterURL,
		binaryURL:   cfg.BinaryURL,
		logger:      logger.Named("ssh-provisioner"),
		connTimeout: connTimeout,
		execTimeout: execTimeout,
	}
}

// SSHProvisionRequest contains the information needed to provision via SSH.
type SSHProvisionRequest struct {
	// AgentID is the unique identifier for the agent
	AgentID string
	// TargetHost is the hostname or IP address
	TargetHost string
	// TargetPort is the SSH port (default 22)
	TargetPort int
	// SSHUser is the username for SSH connection
	SSHUser string
	// SSHKeyName is the name of the stored SSH key to use
	SSHKeyName string
	// Labels are key-value pairs for agent categorization
	Labels map[string]string
	// InstallPath is where to install the agent binary (default /usr/local/bin/vcdeploy-agent)
	InstallPath string
	// ServiceUser is the user the agent service runs as (default vcdeploy)
	ServiceUser string
	// ServiceGroup is the group the agent service runs as (default vcdeploy)
	ServiceGroup string
	// UseSudo indicates whether to use sudo for privileged operations
	UseSudo bool
}

// SSHProvisionResult contains the result of an SSH provisioning operation.
type SSHProvisionResult struct {
	// AgentID is the provisioned agent ID
	AgentID string
	// TargetHost is the target machine
	TargetHost string
	// Status is the final status
	Status string
	// Token is the registration token used
	Token string
	// Output contains the installation output
	Output string
	// StartedAt is when installation started
	StartedAt time.Time
	// CompletedAt is when installation completed
	CompletedAt time.Time
}

// SSHProvision provisions an agent via SSH connection.
// This connects to the target machine, installs the agent binary,
// configures it, and starts the service.
func (p *SSHProvisioner) SSHProvision(ctx context.Context, req *SSHProvisionRequest) (*SSHProvisionResult, error) {
	startTime := time.Now()

	// Validate request
	if req.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if req.TargetHost == "" {
		return nil, fmt.Errorf("target_host is required")
	}
	if req.SSHUser == "" {
		return nil, fmt.Errorf("ssh_user is required")
	}
	if req.SSHKeyName == "" {
		return nil, fmt.Errorf("ssh_key_name is required")
	}

	// Set defaults
	if req.TargetPort == 0 {
		req.TargetPort = 22
	}
	if req.InstallPath == "" {
		req.InstallPath = "/usr/local/bin/vcdeploy-agent"
	}
	if req.ServiceUser == "" {
		req.ServiceUser = "vcdeploy"
	}
	if req.ServiceGroup == "" {
		req.ServiceGroup = "vcdeploy"
	}

	p.logger.Info("Starting SSH provisioning",
		zap.String("agent_id", req.AgentID),
		zap.String("target", fmt.Sprintf("%s:%d", req.TargetHost, req.TargetPort)),
		zap.String("ssh_user", req.SSHUser),
	)

	result := &SSHProvisionResult{
		AgentID:    req.AgentID,
		TargetHost: req.TargetHost,
		StartedAt:  startTime,
	}

	// Get SSH signer for the key
	signer, err := p.ssh.GetSigner(ctx, req.SSHKeyName)
	if err != nil {
		p.logger.Error("Failed to get SSH key", zap.Error(err))
		return nil, fmt.Errorf("get SSH key: %w", err)
	}
	if signer == nil {
		return nil, ErrSSHKeyNotFound
	}

	// Create SSH client config
	config := &ssh.ClientConfig{
		User: req.SSHUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: p.ssh.TrustOnFirstUse(ctx),
		Timeout:         p.connTimeout,
	}

	// Connect to target
	addr := fmt.Sprintf("%s:%d", req.TargetHost, req.TargetPort)
	p.logger.Debug("Connecting to target", zap.String("addr", addr))

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		p.logger.Error("SSH connection failed", zap.Error(err))
		return nil, fmt.Errorf("%w: %v", ErrSSHConnectionFailed, err)
	}
	defer client.Close()

	// Create output buffer
	var outputBuf bytes.Buffer

	// Step 1: Check if agent already installed
	p.logger.Debug("Checking if agent is already installed")
	installed, err := p.checkAgentInstalled(ctx, client, req.InstallPath)
	if err != nil {
		p.logger.Warn("Error checking if agent installed", zap.Error(err))
	}
	if installed {
		result.Status = "already_installed"
		result.CompletedAt = time.Now()
		return result, ErrAgentAlreadyInstalled
	}

	// Step 2: Detect OS and architecture
	p.logger.Debug("Detecting target OS and architecture")
	osType, arch, err := p.detectPlatform(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("detect platform: %w", err)
	}
	outputBuf.WriteString(fmt.Sprintf("Detected platform: %s/%s\n", osType, arch))

	// Step 3: Create the provisioning request
	provReq := &ProvisionRequest{
		AgentID:      req.AgentID,
		Hostname:     req.TargetHost,
		Labels:       req.Labels,
		SSHUser:      req.SSHUser,
		SSHPort:      req.TargetPort,
		InstallPath:  req.InstallPath,
		ServiceUser:  req.ServiceUser,
		ServiceGroup: req.ServiceGroup,
	}

	// Generate token via the underlying provisioner
	provResult, err := p.provisioner.Provision(ctx, provReq)
	if err != nil {
		return nil, fmt.Errorf("generate provisioning data: %w", err)
	}
	result.Token = provResult.Token

	// Step 4: Execute install script remotely
	p.logger.Info("Installing agent on target",
		zap.String("agent_id", req.AgentID),
		zap.String("target", req.TargetHost),
	)

	installOutput, err := p.runInstallScript(ctx, client, provResult.InstallScript, req.UseSudo)
	outputBuf.WriteString(installOutput)
	if err != nil {
		result.Output = outputBuf.String()
		result.Status = "failed"
		result.CompletedAt = time.Now()
		p.logger.Error("Installation failed",
			zap.String("agent_id", req.AgentID),
			zap.Error(err),
		)
		return result, fmt.Errorf("%w: %v", ErrInstallationFailed, err)
	}

	// Step 5: Verify agent is running
	p.logger.Debug("Verifying agent is running")
	time.Sleep(3 * time.Second) // Give the service time to start

	running, err := p.checkAgentRunning(ctx, client, req.UseSudo)
	if err != nil || !running {
		result.Output = outputBuf.String()
		result.Status = "installed_not_running"
		result.CompletedAt = time.Now()
		p.logger.Warn("Agent installed but not running",
			zap.String("agent_id", req.AgentID),
			zap.Error(err),
		)
		return result, nil
	}

	result.Output = outputBuf.String()
	result.Status = "provisioned"
	result.CompletedAt = time.Now()

	p.logger.Info("SSH provisioning completed successfully",
		zap.String("agent_id", req.AgentID),
		zap.String("target", req.TargetHost),
		zap.Duration("duration", result.CompletedAt.Sub(result.StartedAt)),
	)

	return result, nil
}

// checkAgentInstalled checks if the agent binary exists on the target.
func (p *SSHProvisioner) checkAgentInstalled(ctx context.Context, client *ssh.Client, installPath string) (bool, error) {
	session, err := client.NewSession()
	if err != nil {
		return false, err
	}
	defer session.Close()

	cmd := fmt.Sprintf("test -f %s && echo 'exists'", installPath)
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return false, nil // File doesn't exist
	}

	return strings.TrimSpace(string(output)) == "exists", nil
}

// detectPlatform detects the target's OS and architecture.
func (p *SSHProvisioner) detectPlatform(ctx context.Context, client *ssh.Client) (osType, arch string, err error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput("uname -s && uname -m")
	if err != nil {
		return "", "", fmt.Errorf("run uname: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return "", "", fmt.Errorf("unexpected uname output: %s", output)
	}

	// Normalize OS
	osType = strings.ToLower(strings.TrimSpace(lines[0]))

	// Normalize architecture
	rawArch := strings.ToLower(strings.TrimSpace(lines[1]))
	switch rawArch {
	case "x86_64", "amd64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	case "armv7l", "armv6l":
		arch = "arm"
	default:
		arch = rawArch
	}

	return osType, arch, nil
}

// runInstallScript executes the installation script on the target.
func (p *SSHProvisioner) runInstallScript(ctx context.Context, client *ssh.Client, script string, useSudo bool) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	// Set up input/output
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	stdin, err := session.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("get stdin pipe: %w", err)
	}

	// Start shell
	cmd := "bash -s"
	if useSudo {
		cmd = "sudo bash -s"
	}

	if err := session.Start(cmd); err != nil {
		return "", fmt.Errorf("start shell: %w", err)
	}

	// Write script to stdin
	_, err = io.WriteString(stdin, script)
	if err != nil {
		return "", fmt.Errorf("write script: %w", err)
	}
	stdin.Close()

	// Wait for completion
	err = session.Wait()
	output := stdout.String() + stderr.String()

	if err != nil {
		return output, fmt.Errorf("script execution failed: %w\nOutput: %s", err, output)
	}

	return output, nil
}

// checkAgentRunning checks if the agent service is running.
func (p *SSHProvisioner) checkAgentRunning(ctx context.Context, client *ssh.Client, useSudo bool) (bool, error) {
	session, err := client.NewSession()
	if err != nil {
		return false, err
	}
	defer session.Close()

	cmd := "systemctl is-active vcdeploy-agent"
	if useSudo {
		cmd = "sudo " + cmd
	}

	output, err := session.CombinedOutput(cmd)
	status := strings.TrimSpace(string(output))

	return status == "active", err
}

// SSHDeprovision removes an agent from a target machine via SSH.
func (p *SSHProvisioner) SSHDeprovision(ctx context.Context, agentID, targetHost string, targetPort int, sshUser, sshKeyName string, useSudo bool) error {
	if targetPort == 0 {
		targetPort = 22
	}

	p.logger.Info("Starting SSH deprovisioning",
		zap.String("agent_id", agentID),
		zap.String("target", fmt.Sprintf("%s:%d", targetHost, targetPort)),
	)

	// Get SSH signer
	signer, err := p.ssh.GetSigner(ctx, sshKeyName)
	if err != nil {
		return fmt.Errorf("get SSH key: %w", err)
	}
	if signer == nil {
		return ErrSSHKeyNotFound
	}

	// Connect
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: p.ssh.TrustOnFirstUse(ctx),
		Timeout:         p.connTimeout,
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(targetHost, fmt.Sprintf("%d", targetPort)), config)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSSHConnectionFailed, err)
	}
	defer client.Close()

	// Run uninstall commands
	uninstallScript := `
#!/bin/bash
set -e

echo "Stopping vcdeploy-agent service..."
systemctl stop vcdeploy-agent 2>/dev/null || true
systemctl disable vcdeploy-agent 2>/dev/null || true

echo "Removing files..."
rm -f /etc/systemd/system/vcdeploy-agent.service
rm -f /usr/local/bin/vcdeploy-agent
rm -rf /etc/vcdeploy-agent
rm -rf /var/lib/vcdeploy-agent
rm -rf /var/log/vcdeploy-agent

systemctl daemon-reload

echo "vcdeploy-agent uninstalled successfully"
`

	_, err = p.runInstallScript(ctx, client, uninstallScript, useSudo)
	if err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}

	// Also deprovision from master
	if err := p.provisioner.Deprovision(ctx, agentID); err != nil {
		p.logger.Warn("Failed to deprovision from master database",
			zap.String("agent_id", agentID),
			zap.Error(err),
		)
	}

	p.logger.Info("SSH deprovisioning completed",
		zap.String("agent_id", agentID),
		zap.String("target", targetHost),
	)

	return nil
}

// TestSSHConnection tests connectivity to a target host.
func (p *SSHProvisioner) TestSSHConnection(ctx context.Context, targetHost string, targetPort int, sshUser, sshKeyName string) error {
	if targetPort == 0 {
		targetPort = 22
	}

	// Get SSH signer
	signer, err := p.ssh.GetSigner(ctx, sshKeyName)
	if err != nil {
		return fmt.Errorf("get SSH key: %w", err)
	}
	if signer == nil {
		return ErrSSHKeyNotFound
	}

	// Connect
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: p.ssh.TrustOnFirstUse(ctx),
		Timeout:         p.connTimeout,
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(targetHost, fmt.Sprintf("%d", targetPort)), config)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSSHConnectionFailed, err)
	}
	defer client.Close()

	// Run simple command to verify
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	_, err = session.CombinedOutput("echo 'connection test'")
	return err
}
