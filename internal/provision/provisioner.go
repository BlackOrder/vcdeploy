// Package provision handles agent provisioning and lifecycle management.
package provision

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// installScriptTmpl is the pre-parsed installation script template.
var installScriptTmpl = template.Must(template.New("install").Parse(installScriptTemplate))

// Provisioner handles agent provisioning operations.
type Provisioner struct {
	agents    services.AgentServicer
	ca        *security.CAManager
	ssh       *security.SSHKeyManager
	logger    *zap.Logger
	masterURL string

	// tokenCallback is called when a new token is generated for agent registration
	tokenCallback func(agentID, token string)
}

// ProvisionerConfig contains configuration for the provisioner.
type ProvisionerConfig struct {
	MasterURL     string
	TokenCallback func(agentID, token string)
}

// NewProvisioner creates a new agent provisioner.
func NewProvisioner(agents services.AgentServicer, ca *security.CAManager, ssh *security.SSHKeyManager, logger *zap.Logger, cfg ProvisionerConfig) *Provisioner {
	return &Provisioner{
		agents:        agents,
		ca:            ca,
		ssh:           ssh,
		logger:        logger.Named("provisioner"),
		masterURL:     cfg.MasterURL,
		tokenCallback: cfg.TokenCallback,
	}
}

// ProvisionRequest contains the information needed to provision a new agent.
type ProvisionRequest struct {
	// AgentID is the unique identifier for the agent
	AgentID string
	// Hostname is the target machine hostname or IP
	Hostname string
	// Labels are key-value pairs for agent categorization
	Labels map[string]string
	// SSHUser is the SSH username for installation
	SSHUser string
	// SSHPort is the SSH port (default 22)
	SSHPort int
	// InstallPath is where to install the agent binary
	InstallPath string
	// ServiceUser is the user the agent service will run as
	ServiceUser string
	// ServiceGroup is the group the agent service will run as
	ServiceGroup string
}

// ProvisionResult contains the result of a provisioning operation.
type ProvisionResult struct {
	// AgentID is the provisioned agent's ID
	AgentID string
	// Token is the one-time registration token
	Token string
	// InstallScript is the generated installation script
	InstallScript string
	// SSHPublicKey is the SSH public key for the agent
	SSHPublicKey string
	// ExpiresAt is when the registration token expires
	ExpiresAt time.Time
}

// Provision creates a new agent provisioning request.
// This generates registration tokens and installation scripts but doesn't
// actually connect to the target machine.
func (p *Provisioner) Provision(ctx context.Context, req *ProvisionRequest) (*ProvisionResult, error) {
	if req.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if req.Hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}

	// Set defaults
	if req.SSHPort == 0 {
		req.SSHPort = 22
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

	// Check if agent already exists
	existing, err := p.agents.GetByID(ctx, req.AgentID)
	if err != nil && !services.IsNotFound(err) {
		return nil, fmt.Errorf("checking existing agent: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("agent %s already exists", req.AgentID)
	}

	// Generate registration token
	token, err := security.GenerateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	// Generate install script
	script, err := p.generateInstallScript(req, token)
	if err != nil {
		return nil, fmt.Errorf("generating install script: %w", err)
	}

	// Create pending agent record
	agent := &storage.Agent{
		ID:       req.AgentID,
		Hostname: req.Hostname,
		Labels:   req.Labels,
		Status:   "pending",
	}
	if err := p.agents.Upsert(ctx, agent); err != nil {
		return nil, fmt.Errorf("creating agent record: %w", err)
	}

	// Register the token for gRPC registration
	if p.tokenCallback != nil {
		p.tokenCallback(req.AgentID, token)
	}

	p.logger.Info("Provisioned agent",
		zap.String("agent_id", req.AgentID),
		zap.String("hostname", req.Hostname),
	)

	return &ProvisionResult{
		AgentID:       req.AgentID,
		Token:         token,
		InstallScript: script,
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}, nil
}

// Deprovision removes an agent and revokes its certificates.
func (p *Provisioner) Deprovision(ctx context.Context, agentID string) error {
	// Check agent exists
	_, err := p.agents.GetByID(ctx, agentID)
	if services.IsNotFound(err) {
		return fmt.Errorf("agent %s not found", agentID)
	}
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}

	// Revoke the agent's certificate if CA is available
	if p.ca != nil {
		cert, err := p.ca.GetAgentCertificate(ctx, agentID)
		if err != nil {
			p.logger.Warn("Failed to get agent certificate",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
		} else if cert != nil {
			if err := p.ca.RevokeCertificate(ctx, cert.SerialNumber, "agent deprovisioned"); err != nil {
				p.logger.Warn("Failed to revoke certificate",
					zap.String("agent_id", agentID),
					zap.Error(err),
				)
			}
		}
	}

	// Delete the agent record
	if err := p.agents.Delete(ctx, agentID); err != nil {
		return fmt.Errorf("deleting agent: %w", err)
	}

	p.logger.Info("Deprovisioned agent", zap.String("agent_id", agentID))
	return nil
}

// ListAgents returns all provisioned agents.
func (p *Provisioner) ListAgents(ctx context.Context) ([]*storage.Agent, error) {
	return p.agents.List(ctx)
}

// GetAgent returns a specific agent.
func (p *Provisioner) GetAgent(ctx context.Context, agentID string) (*storage.Agent, error) {
	return p.agents.GetByID(ctx, agentID)
}

// UpdateAgentLabels updates an agent's labels.
func (p *Provisioner) UpdateAgentLabels(ctx context.Context, agentID string, labels map[string]string) error {
	agent, err := p.agents.GetByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.Labels = labels
	return p.agents.Upsert(ctx, agent)
}

// RegenerateToken generates a new registration token for an agent.
// This is useful if the original token expired or was compromised.
func (p *Provisioner) RegenerateToken(ctx context.Context, agentID string) (string, error) {
	agent, err := p.agents.GetByID(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("getting agent: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent %s not found", agentID)
	}

	if agent.Status != "pending" {
		return "", fmt.Errorf("agent %s is already registered (status: %s)", agentID, agent.Status)
	}

	token, err := security.GenerateSecureToken(32)
	if err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}

	if p.tokenCallback != nil {
		p.tokenCallback(agentID, token)
	}

	return token, nil
}

// GetInstallScript returns an installation script for an agent.
func (p *Provisioner) GetInstallScript(ctx context.Context, agentID, token string) (string, error) {
	agent, err := p.agents.GetByID(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("getting agent: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent %s not found", agentID)
	}

	req := &ProvisionRequest{
		AgentID:  agentID,
		Hostname: agent.Hostname,
		Labels:   agent.Labels,
	}

	return p.generateInstallScript(req, token)
}

// generateInstallScript creates the installation script from the template.
func (p *Provisioner) generateInstallScript(req *ProvisionRequest, token string) (string, error) {
	data := map[string]interface{}{
		"AgentID":      req.AgentID,
		"MasterURL":    p.masterURL,
		"Token":        token,
		"Hostname":     req.Hostname,
		"InstallPath":  req.InstallPath,
		"ServiceUser":  req.ServiceUser,
		"ServiceGroup": req.ServiceGroup,
		"Labels":       req.Labels,
	}

	var buf bytes.Buffer
	if err := installScriptTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// installScriptTemplate is the bash script template for agent installation.
const installScriptTemplate = `#!/bin/bash
set -e

# vcdeploy Agent Installation Script
# Generated for agent: {{.AgentID}}
# Target host: {{.Hostname}}

AGENT_ID="{{.AgentID}}"
MASTER_URL="{{.MasterURL}}"
TOKEN="{{.Token}}"
INSTALL_PATH="{{.InstallPath}}"
SERVICE_USER="{{.ServiceUser}}"
SERVICE_GROUP="{{.ServiceGroup}}"

echo "Installing vcdeploy agent..."

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root or with sudo"
    exit 1
fi

# Create service user if it doesn't exist
if ! id "$SERVICE_USER" &>/dev/null; then
    echo "Creating service user $SERVICE_USER..."
    useradd --system --shell /usr/sbin/nologin --home-dir /var/lib/vcdeploy-agent "$SERVICE_USER"
fi

# Create directories
echo "Creating directories..."
mkdir -p /etc/vcdeploy-agent
mkdir -p /var/lib/vcdeploy-agent
mkdir -p /var/log/vcdeploy-agent

# Download agent binary
echo "Downloading agent binary..."
curl -fsSL "${MASTER_URL}/api/v1/agent/binary" -o "$INSTALL_PATH"
chmod +x "$INSTALL_PATH"

# Create configuration file
echo "Creating configuration..."
cat > /etc/vcdeploy-agent/config.yaml << EOF
agent_id: $AGENT_ID
master_url: $MASTER_URL
registration_token: $TOKEN
log_level: info
data_dir: /var/lib/vcdeploy-agent
EOF

chmod 600 /etc/vcdeploy-agent/config.yaml
chown -R "$SERVICE_USER:$SERVICE_GROUP" /etc/vcdeploy-agent
chown -R "$SERVICE_USER:$SERVICE_GROUP" /var/lib/vcdeploy-agent
chown -R "$SERVICE_USER:$SERVICE_GROUP" /var/log/vcdeploy-agent

# Create systemd service
echo "Creating systemd service..."
cat > /etc/systemd/system/vcdeploy-agent.service << EOF
[Unit]
Description=vcdeploy Agent
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_GROUP
ExecStart=$INSTALL_PATH -config /etc/vcdeploy-agent/config.yaml
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/lib/vcdeploy-agent /var/log/vcdeploy-agent

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and start service
echo "Starting service..."
systemctl daemon-reload
systemctl enable vcdeploy-agent
systemctl start vcdeploy-agent

# Wait for registration
echo "Waiting for agent to register..."
sleep 5

if systemctl is-active --quiet vcdeploy-agent; then
    echo "✓ vcdeploy agent installed and running successfully!"
    echo "  Agent ID: $AGENT_ID"
    echo "  Status: $(systemctl is-active vcdeploy-agent)"
else
    echo "✗ Agent failed to start. Check logs with: journalctl -u vcdeploy-agent"
    exit 1
fi
`

// UninstallScriptTemplate is the bash script for agent removal.
const UninstallScriptTemplate = `#!/bin/bash
set -e

echo "Uninstalling vcdeploy agent..."

# Stop and disable service
if systemctl is-active --quiet vcdeploy-agent; then
    echo "Stopping service..."
    systemctl stop vcdeploy-agent
fi
systemctl disable vcdeploy-agent 2>/dev/null || true

# Remove files
echo "Removing files..."
rm -f /etc/systemd/system/vcdeploy-agent.service
rm -f {{.InstallPath}}
rm -rf /etc/vcdeploy-agent
rm -rf /var/lib/vcdeploy-agent

# Optionally remove logs
read -p "Remove log files? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf /var/log/vcdeploy-agent
fi

# Reload systemd
systemctl daemon-reload

echo "✓ vcdeploy agent uninstalled successfully!"
`
