# Security Guide

This guide covers vcdeploy's security architecture, including mutual TLS (mTLS) for master-agent communication, certificate management, and authentication flows.

## Overview

vcdeploy uses a defense-in-depth security model:

1. **mTLS (Mutual TLS)** - All master-agent communication requires both parties to present valid certificates
2. **Internal CA** - vcdeploy maintains its own Certificate Authority for issuing agent certificates
3. **KMS (Key Management Service)** - All sensitive data is encrypted at rest using a master key
4. **HMAC Re-authentication** - Agents can re-authenticate using HMAC when certificates expire

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

## See Also

- [Provisioning Guide](provisioning.md) - SSH-based agent provisioning
- [Private Repos Guide](private-repos.md) - Source credential management
- [API Reference](api/) - Security API endpoints
- [CLI Reference](cli/) - Security CLI commands
