# Secrets Management

vcdeploy provides secure secrets management with encryption at rest.

## Overview

Secrets in vcdeploy are:
- Encrypted using AES-256-GCM
- Scoped to global, project, or environment level
- Automatically injected during deployments
- Audited for all access

## Creating Secrets

### Via CLI

```bash
# Global secret
vcdeploy secret set DATABASE_URL "postgres://user:pass@host/db"

# Project secret
vcdeploy secret set DATABASE_URL "postgres://..." --project myapp

# Environment-scoped secret
vcdeploy secret set DATABASE_URL "postgres://..." --project myapp --env production
```

### Via Web UI

1. Navigate to **Secrets**
2. Click **Add Secret**
3. Enter key and value
4. Select scope (global/project/environment)
5. Save

## Secret Scopes

### Global Secrets

Available to all projects:
```bash
vcdeploy secret set DOCKER_TOKEN "dckr_pat_xxx"
```

### Project Secrets

Available only to a specific project:
```bash
vcdeploy secret set API_KEY "abc123" --project myapp
```

### Environment Secrets

Override for specific environments:
```bash
vcdeploy secret set DATABASE_URL "postgres://prod-db/app" --project myapp --env production
vcdeploy secret set DATABASE_URL "postgres://dev-db/app" --project myapp --env development
```

## Using Secrets in Projects

### In Environment Variables

```yaml
# project.yaml
environment:
  DATABASE_URL: "${secret:DATABASE_URL}"
  API_KEY: "${secret:API_KEY}"
```

### In Configuration Files

```yaml
# .env.template
DATABASE_URL=${secret:DATABASE_URL}
REDIS_URL=${secret:REDIS_URL}
```

### In Build Commands

```yaml
build:
  commands:
    - "export NPM_TOKEN=${secret:NPM_TOKEN} && npm ci"
```

## Secret Resolution Order

When a secret is requested, vcdeploy checks:

1. Environment-specific secret (if env specified)
2. Project-specific secret
3. Global secret

First match wins.

## Listing Secrets

```bash
# List all secrets (values hidden)
vcdeploy secret list

# List project secrets
vcdeploy secret list --project myapp

# Output
KEY             SCOPE       PROJECT   UPDATED
DATABASE_URL    global      -         2024-01-15
API_KEY         project     myapp     2024-01-14
REDIS_URL       project     myapp     2024-01-10
```

## Deleting Secrets

```bash
# Delete global secret
vcdeploy secret delete DATABASE_URL

# Delete project secret
vcdeploy secret delete API_KEY --project myapp
```

## Encryption

Secrets are encrypted using:
- Algorithm: AES-256-GCM
- Key derivation: PBKDF2
- Storage: SQLite database

### Encryption Key Management

```yaml
# master.yaml
secrets:
  encryption_key: "your-32-byte-encryption-key"
```

Generate a secure key:
```bash
openssl rand -base64 32
```

### Key Rotation

To rotate the encryption key:
1. Export secrets: `vcdeploy secret export > secrets.json`
2. Update encryption key in config
3. Restart master
4. Import secrets: `vcdeploy secret import secrets.json`
5. Securely delete `secrets.json`

## Audit Logging

All secret access is logged:

```bash
vcdeploy audit list --type secret

# Output
TIME                 USER     ACTION        SECRET        PROJECT
2024-01-15 10:30:00  admin    read          DATABASE_URL  myapp
2024-01-15 10:29:00  deploy   read          API_KEY       myapp
2024-01-14 15:00:00  admin    write         DATABASE_URL  -
```

## Best Practices

1. **Use environment-specific secrets** for different stages
2. **Rotate secrets regularly**, especially after team changes
3. **Use strong, random values** for API keys and tokens
4. **Limit access** to secrets using RBAC
5. **Never commit secrets** to version control
6. **Monitor audit logs** for unusual access patterns

## Integration with External Secret Managers

vcdeploy can integrate with external secret managers:

### HashiCorp Vault

```yaml
secrets:
  backend: "vault"
  vault:
    address: "https://vault.example.com"
    token: "${VAULT_TOKEN}"
    path: "secret/vcdeploy"
```

### AWS Secrets Manager

```yaml
secrets:
  backend: "aws"
  aws:
    region: "us-east-1"
    prefix: "vcdeploy/"
```
