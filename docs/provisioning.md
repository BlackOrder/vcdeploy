# Agent Provisioning Guide

This guide covers provisioning vcdeploy agents on remote servers using SSH.

## Overview

Agent provisioning automates the deployment of vcdeploy agents to remote servers:

1. Connect to target server via SSH
2. Download and install the agent binary
3. Configure the agent with master connection details
4. Register the agent with the master
5. Start the agent service

## Prerequisites

Before provisioning, ensure:

- **SSH Access**: The master can reach the target server via SSH
- **SSH Key**: An SSH key is configured in vcdeploy for authentication
- **Root/Sudo**: SSH user has root or sudo access for installation
- **Network**: Target server can connect back to the master on port 9001

## SSH Key Setup

### Generate a New Key

```bash
# Generate an Ed25519 SSH key
vcdeploy ssh-keys generate --name deploy-key --comment "Agent provisioning"

# Output includes the public key - add this to target servers
```

### Import an Existing Key

```bash
# Import from file
vcdeploy ssh-keys import --name existing-key --file ~/.ssh/id_ed25519

# Import from stdin
cat ~/.ssh/id_ed25519 | vcdeploy ssh-keys import --name piped-key --stdin
```

### List Available Keys

```bash
vcdeploy ssh-keys list
```

Output:
```
ID    NAME          TYPE      FINGERPRINT           CREATED
1     deploy-key    ed25519   SHA256:abc123...      2024-01-15
2     legacy-key    rsa       SHA256:def456...      2024-01-10
```

### Get Public Key

```bash
# Display public key for copying
vcdeploy ssh-keys public 1

# Save to file
vcdeploy ssh-keys public 1 > deploy-key.pub
```

## Provisioning an Agent

### Basic Provisioning

```bash
vcdeploy provision \
  --host 192.168.1.100 \
  --user root \
  --ssh-key 1
```

### With Custom Options

```bash
vcdeploy provision \
  --host web-server-01.example.com \
  --port 2222 \
  --user deploy \
  --ssh-key 1 \
  --agent-id "web-01" \
  --agent-name "Web Server 01" \
  --groups "web,production"
```

### Options Reference

| Option | Description | Default |
|--------|-------------|---------|
| `--host` | Target hostname or IP (required) | - |
| `--port` | SSH port | 22 |
| `--user` | SSH username | root |
| `--ssh-key` | SSH key ID from vcdeploy | - |
| `--agent-id` | Agent identifier | auto-generated |
| `--agent-name` | Display name | hostname |
| `--groups` | Comma-separated group list | - |
| `--no-start` | Don't start agent after install | false |

## Provisioning Process

### What Happens

1. **SSH Connection**
   - Connect to target using specified SSH key
   - Establish secure channel for commands

2. **System Detection**
   - Detect OS (Debian/Ubuntu, RHEL/CentOS, Alpine)
   - Detect architecture (amd64, arm64)
   - Verify system requirements

3. **Binary Download**
   - Download appropriate agent binary
   - Verify checksum for integrity

4. **Installation**
   - Install binary to `/usr/local/bin/vcdeploy-agent`
   - Create configuration in `/etc/vcdeploy/agent.yaml`
   - Set appropriate permissions

5. **Certificate Provisioning**
   - Request certificate from master
   - Install certificate and key
   - Configure CA trust

6. **Service Setup**
   - Create systemd service unit
   - Enable service for auto-start
   - Start the agent (unless `--no-start`)

7. **Registration**
   - Agent connects to master
   - Completes registration handshake
   - Appears in master's agent list

### Directory Structure

After provisioning, the agent has:

```
/usr/local/bin/
└── vcdeploy-agent          # Agent binary

/etc/vcdeploy/
└── agent.yaml              # Configuration

/var/lib/vcdeploy/
├── agent.crt               # Agent certificate
├── agent.key               # Agent private key
└── ca.crt                  # CA trust bundle

/var/log/vcdeploy/
└── agent.log               # Log file

/etc/systemd/system/
└── vcdeploy-agent.service  # Service unit
```

## Monitoring Provisioning

### Check Status

```bash
# Get job status
vcdeploy provision status abc123

# Example output:
Job ID:     abc123
Target:     root@192.168.1.100:22
Status:     running
Progress:   75%
Message:    Installing agent binary

Started:    2024-01-15T10:30:00Z
```

### View Logs

```bash
# Get provisioning logs
vcdeploy provision logs abc123

# Example output:
[10:30:01] INFO: Connecting to 192.168.1.100:22
[10:30:02] INFO: Detecting system information
[10:30:02] INFO: OS: Ubuntu 22.04, Arch: amd64
[10:30:03] INFO: Downloading agent binary
[10:30:10] INFO: Installing to /usr/local/bin/vcdeploy-agent
[10:30:11] INFO: Configuring agent
[10:30:12] INFO: Requesting certificate
[10:30:13] INFO: Installing certificate
[10:30:14] INFO: Creating systemd service
[10:30:15] INFO: Starting agent
[10:30:16] INFO: Agent registered successfully
```

### List Recent Jobs

```bash
# List all provisioning jobs
vcdeploy provision list

# Filter by status
vcdeploy provision list --status failed
```

## Bulk Provisioning

For provisioning multiple servers, use a script:

```bash
#!/bin/bash
HOSTS=(
  "web-01.example.com"
  "web-02.example.com"
  "api-01.example.com"
)

for host in "${HOSTS[@]}"; do
  echo "Provisioning $host..."
  vcdeploy provision \
    --host "$host" \
    --user deploy \
    --ssh-key 1 \
    --groups "production" &
done

wait
echo "All provisioning jobs started"
```

## Troubleshooting

### Connection Issues

**"Connection refused"**
- Verify SSH service is running on target
- Check firewall allows SSH port
- Verify correct hostname/IP

**"Permission denied"**
- Verify SSH key is correct
- Check key is in authorized_keys on target
- Verify user exists and has access

**"Host key verification failed"**
- Target's host key may have changed
- Add host key to master's known_hosts
- Or provision with host key verification disabled (not recommended)

### Authentication Issues

**"Authentication failed"**
- SSH key may be incorrect
- Key may not be in target's authorized_keys
- Key may be encrypted (provide passphrase)

**"Sudo password required"**
- User needs NOPASSWD sudo access
- Or use root user directly

### Installation Issues

**"Binary download failed"**
- Target may not have internet access
- Configure master as binary mirror
- Or pre-stage binary on target

**"Unsupported OS"**
- OS not in supported list
- Try manual installation

**"Permission denied writing files"**
- User lacks root/sudo access
- Verify sudo is configured correctly

### Agent Issues

**"Agent failed to start"**
- Check agent logs: `journalctl -u vcdeploy-agent`
- Verify configuration syntax
- Check certificate files exist

**"Agent not connecting"**
- Verify master is reachable from target
- Check firewall allows port 9001
- Verify certificate is valid

### Recovery

**Re-provision failed agent**
```bash
# Remove existing installation
ssh user@host "sudo systemctl stop vcdeploy-agent; sudo rm -rf /var/lib/vcdeploy"

# Provision again
vcdeploy provision --host host --user user --ssh-key 1
```

**Manually fix certificate**
```bash
# On target server
sudo systemctl stop vcdeploy-agent

# On master
vcdeploy certs revoke agent-id
# Note the new cert from provisioning

# Start agent
sudo systemctl start vcdeploy-agent
```

## Security Considerations

### SSH Key Security

1. **Dedicated Keys**: Use separate keys for provisioning vs. other access
2. **Key Rotation**: Rotate provisioning keys periodically
3. **Limited Access**: Restrict key to provisioning user only
4. **Audit Trail**: All provisioning is logged

### Network Security

1. **Firewall Rules**: Only allow SSH from master
2. **Jump Hosts**: Use bastion hosts for isolated networks
3. **VPN**: Consider VPN for cross-network provisioning

### Post-Provisioning

1. **Remove Provisioning Access**: Optionally remove SSH key after provisioning
2. **Verify Agent**: Confirm agent is registered and healthy
3. **Update Inventory**: Document provisioned servers

## API Reference

### Start Provisioning
```
POST /api/v1/agents/provision
{
  "host": "192.168.1.100",
  "port": 22,
  "user": "root",
  "ssh_key_id": 1,
  "agent_id": "web-01",
  "agent_name": "Web Server 01",
  "groups": ["web", "production"]
}
```

### Get Status
```
GET /api/v1/agents/provision/{job_id}/status
```

### Get Logs
```
GET /api/v1/agents/provision/{job_id}/logs
```

## See Also

- [Security Guide](security.md) - Certificate and authentication details
- [CLI Reference](cli/) - Full CLI documentation
- [API Reference](api/) - REST API documentation
