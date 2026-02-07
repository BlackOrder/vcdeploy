# Secrets Management

vcdeploy provides secure secrets management with a built-in Key Management System (KMS).

## Overview

Secrets in vcdeploy are:
- **Encrypted** using AES-256-GCM with versioned keys
- **Scoped** to project and environment level
- **Audited** with usage logging for compliance
- **Rotatable** with automatic re-encryption support

## Architecture

### Key Management System (KMS)

vcdeploy includes a built-in KMS that manages encryption keys with full lifecycle support:

```
┌─────────────────────────────────────────────┐
│                    KMS                       │
├─────────────────────────────────────────────┤
│  Key Lifecycle:                              │
│  pending → active → inactive → scheduled → deleted │
│                                              │
│  Ciphertext Format:                          │
│  v1:{key_id}:{base64_nonce}:{base64_cipher}  │
│                                              │
│  Algorithm: AES-256-GCM                      │
│  Key Size: 256 bits (32 bytes)               │
└─────────────────────────────────────────────┘
```

### Key States

| State | Can Encrypt | Can Decrypt | Description |
|-------|-------------|-------------|-------------|
| `pending` | No | No | Created but not activated |
| `active` | Yes | Yes | Current key for new encryptions |
| `inactive` | No | Yes | Rotated out, retained for decryption |
| `scheduled` | No | Yes | Scheduled for deletion (grace period) |
| `deleted` | No | No | Logically deleted |

## Secret Scopes

Secrets are organized by project and scope (environment):

```
Project: myapp
├── Scope: production
│   ├── DATABASE_URL
│   └── API_KEY
├── Scope: staging
│   ├── DATABASE_URL
│   └── API_KEY
└── Scope: development
    ├── DATABASE_URL
    └── DEBUG_KEY
```

## CLI Commands

### Setting Secrets

```bash
# Set a secret (prompts for value)
vcdeploy secret set myapp/production DATABASE_URL

# Set from stdin (piped value)
echo "postgres://user:pass@host/db" | vcdeploy secret set myapp/production DATABASE_URL --stdin

# Set with inline value (not recommended for sensitive data)
vcdeploy secret set myapp/production DATABASE_URL "postgres://user:pass@host/db"
```

### Listing Secrets

```bash
# List secrets for a project
vcdeploy secret list myapp

# Output:
# KEY             SCOPE        UPDATED
# DATABASE_URL    production   2024-01-15
# API_KEY         production   2024-01-14
# DATABASE_URL    staging      2024-01-10
```

### Deleting Secrets

```bash
# Delete a specific secret
vcdeploy secret delete myapp/production DATABASE_URL
```

### Import/Export

```bash
# Import secrets from .env file format
cat secrets.env | vcdeploy secret import myapp/production

# Backup all secrets (passphrase protected)
vcdeploy secret backup -o secrets-backup.enc

# Restore secrets from backup
vcdeploy secret restore secrets-backup.enc
```

## Using Secrets in Projects

### Environment Variable Substitution

In your project configuration, reference secrets using the `${secret:NAME}` syntax:

```yaml
# project.yaml
env:
  template: ".env.template"
  placeholder_pattern: "${SECRET_NAME}"  # Default pattern
  required_keys:
    - "DATABASE_URL"
    - "API_KEY"
```

Your `.env.template` file:
```bash
# Application settings
APP_NAME=MyApp
APP_ENV=production

# Substituted at deploy time
DATABASE_URL=${DATABASE_URL}
REDIS_URL=${REDIS_URL}
API_KEY=${API_KEY}
```

During deployment, vcdeploy:
1. Reads the template file
2. Looks up each `${SECRET_NAME}` in the project's scope
3. Decrypts and substitutes the values
4. Writes the final `.env` file to the deployment
5. Captures an encrypted env snapshot for this deployment

> **Env Snapshots:** Secrets resolved at deployment time are captured in an encrypted env snapshot. On rollback, the snapshot from the original deployment is restored, ensuring the correct environment is used regardless of current secret values.

### Secret Resolution

Secrets are resolved by project and scope:

```bash
# Request: Get "DATABASE_URL" for project "myapp", scope "production"
# vcdeploy looks up: myapp -> production -> DATABASE_URL
```

## Key Rotation

### Rotating the Master Key

Regular key rotation improves security:

```bash
# Rotate the encryption key
vcdeploy master rotate-key
```

This command:
1. Generates a new encryption key
2. Sets the new key as active
3. Marks the old key as inactive (retained for decryption)
4. All new encryptions use the new key
5. Old secrets remain decryptable

### Re-encrypting Secrets

After key rotation, you can re-encrypt all secrets with the new key:

```bash
# Re-encrypt all secrets with current key
vcdeploy secret re-encrypt
```

### Key Deletion

Keys can be scheduled for deletion with a grace period:

```bash
# Schedule key deletion (30-day grace period by default)
vcdeploy master key delete KEY_ID

# Cancel scheduled deletion
vcdeploy master key cancel-delete KEY_ID

# Force immediate deletion (requires --confirm-destroy)
vcdeploy master key delete KEY_ID --confirm-destroy
```

> **Warning**: Deleted keys cannot decrypt data. Ensure all secrets are re-encrypted before deleting old keys.

## API Endpoints

### Secrets API

```bash
# List secrets for a project (use project ID)
GET /api/v1/projects/{id}/secrets

# Set a secret
POST /api/v1/projects/{id}/secrets
Content-Type: application/json
{
  "scope": "production",
  "key": "DATABASE_URL",
  "value": "postgres://..."
}

# Delete a secret
DELETE /api/v1/projects/{id}/secrets/{scope}/{key}
```

### Key Management API

```bash
# List encryption keys
GET /api/v1/keys

# Rotate key
POST /api/v1/keys/rotate

# Get key info
GET /api/v1/keys/{key_id}

# Schedule key deletion
DELETE /api/v1/keys/{key_id}
```

## Database Storage

### Secrets Table

```sql
CREATE TABLE secrets (
    id INTEGER PRIMARY KEY,
    project TEXT NOT NULL,
    project_id INTEGER,           -- FK to projects (optional)
    scope TEXT NOT NULL,          -- Environment name
    key TEXT NOT NULL,
    value_encrypted BLOB NOT NULL,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    UNIQUE(project, scope, key)
);
```

### Encryption Keys Table

```sql
CREATE TABLE encryption_keys (
    id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    key_material_encrypted BLOB NOT NULL,
    algorithm TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP,
    activated_at TIMESTAMP,
    deactivated_at TIMESTAMP,
    scheduled_deletion_at TIMESTAMP,
    deletion_cancelled_at TIMESTAMP
);
```

### Key Usage Audit

```sql
CREATE TABLE encryption_key_usage (
    id INTEGER PRIMARY KEY,
    key_id TEXT NOT NULL,
    operation TEXT NOT NULL,      -- encrypt, decrypt
    resource_type TEXT,
    resource_id TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Security Best Practices

### 1. Regular Key Rotation

Rotate encryption keys periodically:

```bash
# Rotate monthly
0 0 1 * * vcdeploy master rotate-key
```

### 2. Use Environment-Specific Secrets

Don't share secrets across environments:

```bash
# Production
vcdeploy secret set myapp/production DATABASE_URL "postgres://prod-db/app"

# Staging (different credentials)
vcdeploy secret set myapp/staging DATABASE_URL "postgres://staging-db/app"
```

### 3. Limit Access

Use RBAC to control secret access:
- **Admins**: Full secret management
- **Users**: Set/view secrets for their projects
- **Viewers**: No secret access

### 4. Audit Access

Monitor secret operations:

```bash
vcdeploy audit list --type secret

# Output:
# TIME                 USER     ACTION   KEY           PROJECT
# 2024-01-15 10:30:00  admin    read     DATABASE_URL  myapp
# 2024-01-14 15:00:00  deploy   read     API_KEY       myapp
```

### 5. Secure Backups

Always use strong passphrases for secret backups:

```bash
# Create encrypted backup
vcdeploy secret backup -o /secure/backup/secrets.enc

# Store passphrase separately from backup
```

### 6. Never Log Secrets

vcdeploy never logs decrypted secret values. Ensure your application follows the same practice.

## Troubleshooting

### "No active encryption key"

The KMS hasn't been initialized:

```bash
# Initialize the KMS (done automatically on first start)
vcdeploy master start
```

### "Key not found"

The encryption key used to encrypt the data no longer exists:

```bash
# List all keys to verify
vcdeploy master key list

# If key was deleted, data cannot be recovered
```

### "Cannot decrypt"

1. Verify the key is in `active` or `inactive` state
2. Check key hasn't been deleted
3. Ensure the ciphertext format is valid (`v1:...`)

### Secret Not Substituted

If `${SECRET_NAME}` appears in your deployed `.env`:

1. Verify the secret exists: `vcdeploy secret list myapp`
2. Check the scope matches your target
3. Ensure the key name matches exactly (case-sensitive)

## Migration from Plain Text

If upgrading from an earlier version with unencrypted secrets:

```bash
# Export existing secrets
vcdeploy secret export --format env > secrets.env

# Re-import with encryption
vcdeploy secret import myapp/production < secrets.env
```

## Ciphertext Format

The versioned ciphertext format ensures backward compatibility:

```
v1:{key_id}:{base64_nonce}:{base64_ciphertext}
```

- `v1`: Format version (allows future algorithm changes)
- `key_id`: Identifies which key to use for decryption
- `nonce`: Unique per encryption (prevents replay)
- `ciphertext`: AES-256-GCM encrypted data

Example:
```
v1:abc123def456:MTIzNDU2Nzg5MDEy:SGVsbG8gV29ybGQh...
```

## Comparison with External Systems

vcdeploy's built-in KMS is suitable for most deployments. For specific compliance requirements, consider:

| Feature | vcdeploy KMS | External KMS |
|---------|--------------|--------------|
| Setup | Built-in | Additional infrastructure |
| Key rotation | Supported | Provider-dependent |
| Audit logging | Built-in | Provider-dependent |
| HSM support | No | Available |
| FIPS compliance | No | Available |
| Multi-region | No | Available |

For environments requiring HSM-backed keys or FIPS compliance, you may need to integrate an external KMS solution at the application level.

## Related Documentation

- [Master Configuration](config/master.md) - KMS settings
- [Project Configuration](config/projects.md) - Using secrets in projects
- [API Reference](api/README.md) - Secrets API endpoints
