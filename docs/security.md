# Security Guide

This guide covers vcdeploy's security architecture, including mutual TLS (mTLS) for master-agent communication, certificate management, authentication flows, and two-factor authentication.

## Overview

vcdeploy uses a defense-in-depth security model:

1. **mTLS (Mutual TLS)** - All master-agent communication requires both parties to present valid certificates
2. **Internal CA** - vcdeploy maintains its own Certificate Authority for issuing agent certificates
3. **KMS (Key Management Service)** - All sensitive data is encrypted at rest using a master key
4. **HMAC Re-authentication** - Agents can re-authenticate using HMAC when certificates expire
5. **Two-Factor Authentication (2FA)** - Optional TOTP-based 2FA for user accounts

## Two-Factor Authentication (TOTP)

vcdeploy supports TOTP (Time-based One-Time Password) two-factor authentication for enhanced account security.

### Enabling 2FA

Users can enable 2FA from their profile page:

1. Navigate to your profile by clicking your username in the navigation bar
2. Click "Enable Two-Factor Authentication"
3. Scan the QR code with an authenticator app (Google Authenticator, Authy, etc.)
4. Enter the 6-digit code from your app to verify
5. Save your recovery codes securely

### Recovery Codes

When 2FA is enabled, you receive 8 single-use recovery codes. These codes:

- Can be used instead of your TOTP code if you lose access to your authenticator
- Are shown only once during setup - save them securely!
- Can be regenerated from your profile page (requires current TOTP verification)
- Are formatted as `XXXX-XXXX` for easy reading

**Important:** Store recovery codes in a secure location like a password manager. Each code can only be used once.

### Regenerating Recovery Codes

If you've used some recovery codes or want fresh ones:

1. Go to your profile page
2. Click "Regenerate Codes"
3. Enter your current TOTP code
4. Save the new codes (old codes are invalidated)

### Disabling 2FA

You can disable 2FA from your profile page by entering your current TOTP code.

### Admin Account Recovery

If a user loses access to both their authenticator and all recovery codes, an administrator can disable their 2FA:

**Via CLI:**
```bash
vcdeploy totp disable --user <username> --reason "Lost device, verified via video call" --confirm
```

**Via Admin UI:**
1. Go to Settings → Users
2. Click "Disable 2FA" next to the user
3. Enter a reason for audit purposes
4. If you have 2FA enabled, enter your own TOTP code
5. Click "Disable 2FA"

All admin 2FA disable actions are logged for audit compliance.

### Configuration

Enable 2FA requirements in your master configuration:

```yaml
security:
  require_2fa_admin: true   # Require 2FA for admin users
  require_2fa: false        # Require 2FA for all users
```

## mTLS Architecture

### How It Works

```
┌─────────────────┐                      ┌─────────────────┐
│     Master      │  ◄───── mTLS ──────► │      Agent      │
│                 │                      │                 │
│  • CA Manager   │                      │  • Agent Cert   │
│  • Trust Pool   │                      │  • CA Trust     │
│  • Server Cert  │                      │  • HMAC Secret  │
└─────────────────┘                      └─────────────────┘
         │
         │
         ▼
┌─────────────────┐
│    Database     │
│                 │
│  • CA certs     │
│  • Agent certs  │
│  • Revocations  │
└─────────────────┘
```

### Connection Flow

1. Agent connects to master over TLS
2. Both parties exchange certificates during TLS handshake
3. Master verifies agent certificate against trust pool
4. Agent verifies master certificate against embedded CA
5. If valid, gRPC communication proceeds

### Trust Establishment

The trust pool contains all CA certificates (current and historical) so that:
- New agents get certificates from the current CA
- Existing agents' certificates remain valid after CA rotation
- Revoked certificates are rejected regardless of CA

## Certificate Lifecycle

### Agent Certificates

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│  Issue   │ ──► │  Active  │ ──► │  Renew   │ ──► │  Active  │
└──────────┘     └──────────┘     └──────────┘     └──────────┘
                      │                                  │
                      ▼                                  ▼
                ┌──────────┐                      ┌──────────┐
                │  Revoke  │                      │  Expire  │
                └──────────┘                      └──────────┘
                      │                                  │
                      ▼                                  ▼
                ┌──────────┐                      ┌──────────┐
                │ Revoked  │                      │ Expired  │
                └──────────┘                      └──────────┘
```

#### Issuance

When a new agent is registered, the master issues a certificate:

1. Agent sends registration request with ID and hostname
2. Master generates a key pair for the agent
3. Master signs certificate with current CA
4. Certificate and private key are returned to agent
5. Certificate is stored in database with metadata

#### Renewal

Certificates are automatically renewed before expiration:

1. Renewal threshold is configurable (default: 6 months before expiry)
2. Agent requests renewal via gRPC
3. New certificate is issued, old one remains valid until expiry
4. Agent starts using new certificate

#### Revocation

Certificates can be revoked immediately:

```bash
# Via CLI
vcdeploy certs revoke agent-001 --reason "Decommissioned"

# Via API
POST /api/v1/certificates/agents/agent-001/revoke
{
  "reason": "Decommissioned"
}
```

Revocation:
- Marks certificate as revoked in database
- Agent can no longer authenticate with that certificate
- Logged in audit trail

### CA Certificates

#### Rotation

CA rotation creates a new CA while preserving trust:

1. New CA is generated with configurable validity
2. Old CA is marked inactive (cannot sign new certs)
3. Both CAs remain in trust pool
4. New agents get certificates from new CA
5. Existing agents remain authenticated

```bash
# Rotate CA with 365-day validity
vcdeploy certs ca rotate --validity-days 365
```

#### Trust Pool

The trust pool contains:
- Current (active) CA certificate
- All historical CA certificates
- Allows gradual migration during CA rotation

## HMAC Re-authentication

When an agent's certificate expires before renewal, HMAC provides fallback authentication.

### Flow

```
┌──────────────────────────────────────────────────────────────┐
│                      HMAC Re-auth Flow                       │
└──────────────────────────────────────────────────────────────┘

Agent                                                    Master
  │                                                        │
  │  1. Connect with expired/no cert                       │
  │  ─────────────────────────────────────────────────────>│
  │                                                        │
  │  2. Challenge (nonce)                                  │
  │  <─────────────────────────────────────────────────────│
  │                                                        │
  │  3. HMAC(secret, nonce + agent_id + timestamp)         │
  │  ─────────────────────────────────────────────────────>│
  │                                                        │
  │  4. Verify HMAC, check timestamp window                │
  │                                                        │
  │  5. Issue new certificate                              │
  │  <─────────────────────────────────────────────────────│
  │                                                        │
```

### HMAC Secret

Each agent has a unique HMAC secret:
- Generated during initial registration
- Stored encrypted in master database
- Agent stores it securely (not in config file)
- Used only as fallback when cert auth fails

### Security Considerations

- HMAC has limited validity window (5 minutes by default)
- Replay attacks prevented by nonce + timestamp
- Secret rotation supported but not automatic
- Should only be used for recovery, not regular auth

## Key Management Service (KMS)

### Master Key

The KMS protects all sensitive data:

1. **Master Key Generation**: AES-256 key generated on first startup
2. **Key Storage**: Master key encrypted with passphrase, stored in database
3. **Key Derivation**: Per-purpose keys derived from master key

### What's Encrypted

- CA private keys
- Agent certificate private keys
- Source credentials (passwords, tokens)
- SSH private keys
- HMAC secrets

### Encryption Format

```
v1:{key_version}:{nonce}:{ciphertext}
```

- Version prefix enables key rotation
- Nonce ensures unique ciphertext
- AES-256-GCM provides authenticated encryption

## Security Best Practices

### Certificate Management

1. **Monitor Expiration**: Set up alerts for certificates expiring within threshold
2. **Regular Rotation**: Rotate CA annually even if not required
3. **Prompt Revocation**: Revoke certificates immediately when agents are decommissioned
4. **Audit Logging**: Review certificate audit log regularly

```bash
# List certificates expiring within 30 days
vcdeploy certs list --expiring 30d

# View audit log
vcdeploy certs audit --limit 100
```

### Network Security

1. **Firewall**: Restrict master port (9090) to agent networks only
2. **TLS Version**: TLS 1.2+ enforced, TLS 1.3 preferred
3. **Cipher Suites**: Only modern, secure cipher suites allowed
4. **Certificate Validation**: Never disable certificate validation

### Credential Security

1. **Least Privilege**: Use minimal permissions for source credentials
2. **Separate Credentials**: Different credentials for different repositories
3. **Regular Rotation**: Rotate credentials periodically
4. **Audit Usage**: Review credential usage in logs

## Troubleshooting

### Certificate Errors

**"certificate signed by unknown authority"**
- Agent doesn't have master's CA in trust store
- Re-provision agent or update CA trust

**"certificate has expired"**
- Certificate past NotAfter date
- Use HMAC re-authentication to get new cert
- Check why auto-renewal didn't occur

**"certificate revoked"**
- Certificate was explicitly revoked
- Issue new certificate after verification

### Connection Issues

**Agent can't connect**
1. Check network connectivity
2. Verify firewall rules
3. Check certificate validity
4. Review master logs for TLS errors

**TLS handshake failed**
1. Verify both certificates are valid
2. Check CA trust on both sides
3. Ensure no certificate revocation
4. Check TLS version compatibility

### HMAC Issues

**HMAC verification failed**
1. Check agent's HMAC secret matches master
2. Verify system clocks are synchronized
3. Check if secret was rotated

## Audit Trail

All security operations are logged:

| Event | Description |
|-------|-------------|
| `cert_issued` | New certificate issued to agent |
| `cert_renewed` | Certificate renewed for agent |
| `cert_revoked` | Certificate revoked with reason |
| `ca_rotated` | CA certificate rotated |
| `hmac_auth` | HMAC re-authentication used |
| `auth_failed` | Authentication attempt failed |

Query audit log:
```bash
vcdeploy certs audit --agent agent-001 --limit 50
```

## Configuration Reference

### TLS Configuration Modes

vcdeploy supports three TLS modes for the HTTP/API server:

#### Disabled Mode

```yaml
server:
  tls:
    mode: disabled
```

Not recommended for production. API endpoints are served over plain HTTP.

#### Static Mode

Use your own certificates (e.g., from a corporate CA or purchased from a provider):

```yaml
server:
  tls:
    mode: static
    cert_file: "/path/to/server.crt"
    key_file: "/path/to/server.key"
    force_https: true  # Redirect HTTP to HTTPS
```

The certificate files must exist and be readable. The server will reload certificates on each request, allowing certificate updates without restart.

#### ACME Mode (Let's Encrypt)

Automatic certificate management using ACME protocol:

```yaml
server:
  tls:
    mode: acme
    force_https: true
    acme:
      domains:
        - "vcdeploy.example.com"
        - "www.vcdeploy.example.com"
      email: "admin@example.com"
      staging: false  # Use true for testing
```

Requirements:
- Server must be publicly accessible on port 80 (for HTTP-01 challenge)
- DNS must resolve the configured domains to the server

Certificates are automatically renewed when within 30 days of expiry.

### Certificate Rotation

#### ACME Certificate Rotation

Certificates are automatically renewed by the ACME client. No manual intervention required.

Monitor renewal status via API:
```bash
curl -s https://vcdeploy.example.com/api/v1/tls/status | jq
```

#### Static Certificate Rotation

To update static certificates:
1. Replace the certificate and key files
2. The server will automatically use the new files on the next request

#### CA Rotation

Rotate the internal CA used for agent certificates:
```bash
vcdeploy certs ca rotate --validity-days 3650
```

This preserves existing agent connections by keeping old CA in the trust pool.

### Master Configuration

```yaml
security:
  # CA certificate validity (default: 10 years)
  ca_validity: "87600h"
  
  # Agent certificate validity (default: 1 year)
  cert_validity: "8760h"
  
  # Auto-renewal threshold (default: 6 months)
  renewal_threshold: "4380h"
  
  # HMAC authentication window (default: 5 minutes)
  hmac_window: "5m"
  
  # TLS minimum version (default: TLS 1.2)
  tls_min_version: "1.2"
```

### Agent Configuration

```yaml
security:
  # Path to certificate file
  cert_file: "/var/lib/vcdeploy/agent.crt"
  
  # Path to private key file
  key_file: "/var/lib/vcdeploy/agent.key"
  
  # Path to CA trust file
  ca_file: "/var/lib/vcdeploy/ca.crt"
```

## Command Validation

vcdeploy validates all commands before execution to prevent shell injection attacks.

### Validated Command Types

| Type | Description | Validation |
|------|-------------|------------|
| `shell` | Pre-defined shell commands | Strict allowlist |
| `reload` | Service reload operations | Service name validation |
| `file_operation` | File system operations | Path traversal prevention |
| `raw` | Arbitrary commands | Requires admin approval |

### Validation Rules

1. **No Shell Metacharacters**: Commands are validated to prevent injection
2. **Path Sanitization**: All file paths are validated against traversal attacks
3. **Agent ID Validation**: Agent identifiers follow strict alphanumeric patterns
4. **Hostname Validation**: Hostnames must be valid RFC 1123 format

### RAW Command Security

RAW commands bypass normal validation and require special handling:

1. **Admin Approval Required**: No RAW command executes without admin approval
2. **Approval Audit**: All approvals are logged with admin ID and timestamp
3. **Approval Notes**: Admins can document why the RAW command is safe
4. **One-Time Review**: Re-approval needed if RAW content changes

### Best Practices

1. **Prefer Typed Commands**: Use `shell`, `reload`, or `file_operation` over `raw`
2. **Variable Injection Safety**: Variables are escaped before command substitution
3. **Audit RAW Usage**: Regularly review RAW commands and their approvals
4. **Principle of Least Privilege**: Only grant RAW approval authority to trusted admins

## Recipe System Security

The recipe system includes security features for deployment composition.

### Namespace Isolation

- **Seed Components**: Read-only, system-provided, audited
- **User Components**: Fully editable, project-specific

### Variable Binding Security

Variable bindings support secure value sources:

| Source | Description | Security |
|--------|-------------|----------|
| `literal` | Static value | Visible in config |
| `env` | Environment variable | Server-side only |
| `secret` | Encrypted secret | KMS-protected |

### Dependency Tracking

The system prevents accidental credential deletion:

1. **Pre-Delete Checks**: Warnings before deleting secrets used by playbooks
2. **Dependency Reports**: View all playbooks using a specific secret
3. **Cascade Prevention**: Deleting in-use secrets requires acknowledgment

### Export/Import Security

- **Sensitive Data Exclusion**: Exports don't include secret values
- **Import Validation**: Imported recipes are validated before activation
- **Dry-Run Mode**: Preview import effects without changes

## CLI Access Security

The vcdeploy CLI uses a unified execution model with automatic mode detection and secure local access.

### Execution Modes

The CLI operates in one of two modes:

| Mode | When Used | Authentication |
|------|-----------|----------------|
| **API Mode** | Server is running (local or remote) | Token or Unix socket |
| **Direct Mode** | Server is offline (uses `--offline` flag) | System permissions |

### Mode Detection

The CLI automatically detects the appropriate mode:

1. If `--master` flag or `VCDEPLOY_MASTER` env is set → **API mode (remote)**
2. If local server is running → **API mode (local via Unix socket)**
3. If `--offline` flag is set → **Direct mode**
4. Otherwise → **Direct mode** (requires permissions)

### Unix Socket Access

For local CLI access, the master server exposes a Unix socket at `/var/run/vcdeploy/vcdeploy.sock`. This provides secure local access without requiring API tokens.

**Socket Permissions:**
- Owner: `root`
- Group: `vcdeploy`
- Mode: `0660` (read/write for owner and group only)

### Permission Model

| User Type | Server Running | Server Offline | Auth Required |
|-----------|----------------|----------------|---------------|
| `root` / `sudo` | Unix socket | Direct DB | No |
| `vcdeploy` group member | Unix socket | Direct DB | No |
| Other local users | TCP + token | Permission denied | Yes (token) |
| Remote users | TCP + token | N/A | Yes (token) |

### Granting CLI Access

To allow a non-root user to run CLI commands without an API token:

```bash
# Add user to the vcdeploy group
sudo usermod -aG vcdeploy <username>

# User must log out and back in for group membership to take effect
```

### Security Considerations

1. **No Token Files**: vcdeploy does not store tokens in files on the filesystem, preventing accidental token exposure.

2. **Unix Socket Security**: The socket's group ownership restricts access to authorized users only.

3. **Root Always Allowed**: Root (UID 0) can always access the CLI, regardless of group membership.

4. **Direct Mode Restrictions**: Direct database access (offline mode) is only available to root and vcdeploy group members.

5. **Clear Error Messages**: Non-privileged users receive clear instructions:
   ```
   Error: permission denied: CLI direct access requires root or membership in the vcdeploy group
   Hint: Either:
     1. Run as root/sudo
     2. Add your user to the vcdeploy group: sudo usermod -aG vcdeploy $USER
     3. Use API mode: vcdeploy --master localhost:9000 --token <token> <command>
   ```

### Installation

The post-installation script automatically:

1. Creates the `vcdeploy` system group
2. Creates the Unix socket directory at `/var/run/vcdeploy/`
3. Sets appropriate permissions on directories

## See Also

- [Recipe System Guide](recipes.md) - Full recipe system documentation
- [Provisioning Guide](provisioning.md) - SSH-based agent provisioning
- [Private Repos Guide](private-repos.md) - Source credential management
- [API Reference](api/) - Security API endpoints
- [CLI Reference](cli/) - Security CLI commands

