# Private Repository Guide

This guide covers configuring vcdeploy to deploy from private Git repositories.

## Overview

vcdeploy supports private repositories through source credentials:

- **Basic Auth**: Username and password for HTTPS
- **Personal Access Token**: GitHub, GitLab, Bitbucket tokens
- **SSH Keys**: For SSH-based Git access
- **Deploy Keys**: Repository-specific SSH keys

**Important**: Credentials are stored only on the master server, never on agents. The master clones repositories and streams code to agents securely over mTLS.

## How It Works

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Git Host  │     │   Master    │     │   Agent     │
│  (Private)  │     │             │     │             │
│             │     │             │     │             │
│             │ ◄── │  1. Clone   │     │             │
│             │     │    (creds)  │     │             │
│             │     │             │     │             │
│             │     │  2. Archive │ ──► │  3. Extract │
│             │     │    (mTLS)   │     │    & Deploy │
└─────────────┘     └─────────────┘     └─────────────┘
```

1. Master uses credentials to clone from private repository
2. Code is archived and streamed to agent over secure mTLS connection
3. Agent extracts and deploys without ever seeing credentials

## Credential Types

### Basic Auth

For repositories requiring username/password:

```bash
vcdeploy creds add \
  --name gitlab-creds \
  --type basic \
  --url-pattern "gitlab.company.com/*" \
  --username deploy-user
# You'll be prompted for password
```

Best for:
- Self-hosted GitLab
- Bitbucket Server
- Any HTTPS Git server with basic auth

### Personal Access Token

For GitHub, GitLab, or Bitbucket cloud:

```bash
# GitHub
vcdeploy creds add \
  --name github-token \
  --type token \
  --url-pattern "github.com/myorg/*"
# Paste token when prompted

# GitLab
vcdeploy creds add \
  --name gitlab-token \
  --type token \
  --url-pattern "gitlab.com/myorg/*"
```

Token permissions needed:
- **GitHub**: `repo` scope
- **GitLab**: `read_repository` scope
- **Bitbucket**: Repository read access

### SSH Keys

For SSH-based repository access:

```bash
# First, create or import an SSH key
vcdeploy ssh-keys generate --name git-deploy

# Then create credential referencing the key
vcdeploy creds add \
  --name github-ssh \
  --type ssh \
  --url-pattern "git@github.com:myorg/*" \
  --ssh-key-id 1
```

Add the public key to your Git host:
```bash
vcdeploy ssh-keys public 1
# Copy output to GitHub/GitLab SSH keys
```

### Deploy Keys

Repository-specific SSH keys (more secure):

```bash
# Generate key specifically for this repo
vcdeploy ssh-keys generate --name myrepo-deploy --comment "myorg/myrepo deploy key"

# Create credential
vcdeploy creds add \
  --name myrepo-key \
  --type deploy-key \
  --url-pattern "git@github.com:myorg/myrepo.git" \
  --ssh-key-id 2
```

Then add the public key as a deploy key in your repository settings.

## URL Pattern Matching

Credentials are matched to repositories using URL patterns.

### Pattern Syntax

| Pattern | Matches |
|---------|---------|
| `github.com/*` | Any GitHub HTTPS URL |
| `github.com/myorg/*` | Any repo in myorg |
| `github.com/myorg/myrepo.git` | Exact repo match |
| `git@github.com:myorg/*` | SSH URLs for myorg |
| `*.company.com/*` | Any subdomain |

### Matching Rules

1. More specific patterns take precedence
2. Exact matches win over wildcards
3. First match is used if patterns have same specificity

### Examples

```bash
# Broad pattern for all company repos
vcdeploy creds add \
  --name company-token \
  --type token \
  --url-pattern "github.com/company/*"

# Specific pattern for sensitive repo (higher priority)
vcdeploy creds add \
  --name secrets-repo-key \
  --type deploy-key \
  --url-pattern "github.com/company/secrets.git" \
  --ssh-key-id 3
```

## Managing Credentials

### List Credentials

```bash
vcdeploy creds list

# Output:
ID    NAME             TYPE    URL PATTERN              USAGE
1     github-token     token   github.com/myorg/*       42
2     gitlab-deploy    ssh     git@gitlab.com:corp/*    15
3     bitbucket-creds  basic   bitbucket.org/team/*     8
```

### Test Credentials

Before using in production, test credentials work:

```bash
vcdeploy creds test 1 https://github.com/myorg/private-repo.git

# Output:
Testing credential 1 against https://github.com/myorg/private-repo.git...
✓ Authentication successful
  Repository accessible, 156 commits
```

### Delete Credentials

```bash
vcdeploy creds delete 1

# With confirmation skip
vcdeploy creds delete 1 --force
```

## Project Configuration

### Automatic Matching

When creating a project, vcdeploy automatically matches credentials:

```bash
vcdeploy project create myproject \
  --repo https://github.com/myorg/private-repo.git \
  --branch main \
  --path /var/www/myapp

# vcdeploy will find matching credential automatically
```

### Verify Credential Association

```bash
vcdeploy project show myproject

# Shows:
Name: myproject
Repository: https://github.com/myorg/private-repo.git
Credential: github-token (ID: 1)
```

## Security Best Practices

### Credential Management

1. **Least Privilege**: Use minimal permissions for tokens
2. **Separate Credentials**: Different credentials for different repos/orgs
3. **Regular Rotation**: Rotate tokens periodically
4. **Deploy Keys**: Prefer deploy keys over personal tokens
5. **Audit Trail**: Review credential usage regularly

### Token Scopes

**GitHub Personal Access Token**:
- ✅ `repo` (for private repos)
- ❌ `admin:*` (not needed)
- ❌ `write:*` (not needed for read-only)

**GitLab Personal Access Token**:
- ✅ `read_repository`
- ❌ `api` (too broad)
- ❌ `write_repository` (not needed)

**Bitbucket App Password**:
- ✅ Repository: Read
- ❌ Account: * (not needed)

### SSH Key Security

1. **Ed25519 Keys**: Prefer Ed25519 over RSA
2. **No Passphrase**: Keys are encrypted at rest by vcdeploy
3. **Unique Keys**: One key per purpose/repository
4. **Deploy Keys**: Use deploy keys instead of user keys when possible

## Troubleshooting

### Authentication Failures

**"Repository not found"**
- Credential may lack access to this repo
- Check token scopes/permissions
- Verify URL pattern matches

**"Invalid credentials"**
- Token may be expired or revoked
- Password may have changed
- SSH key not added to Git host

**"Permission denied (publickey)"**
- SSH key not added to Git host
- Wrong SSH key selected
- Key may be revoked

### Debugging

```bash
# Test credential explicitly
vcdeploy creds test CRED_ID REPO_URL

# Check which credential matches
vcdeploy creds list
# Look at URL patterns

# View deployment logs for auth errors
vcdeploy deploy logs DEPLOY_ID | grep -i auth
```

### Common Issues

**Wrong credential matched**
- Check URL patterns overlap
- More specific patterns should exist for sensitive repos
- Remove or update conflicting credentials

**SSH host key verification failed**
- Add Git host's SSH key to known_hosts
- Verify host key hasn't changed

**Token expired**
- GitHub/GitLab tokens have expiry
- Regenerate and update credential

## API Reference

### Create Credential
```
POST /api/v1/credentials
{
  "name": "github-token",
  "type": "token",
  "url_pattern": "github.com/myorg/*",
  "secret": "ghp_..."
}
```

### List Credentials
```
GET /api/v1/credentials
```

### Test Credential
```
POST /api/v1/credentials/{id}/test
{
  "repo_url": "https://github.com/myorg/repo.git"
}
```

### Delete Credential
```
DELETE /api/v1/credentials/{id}
```

## Migration from Other Systems

### From Environment Variables

If you previously used environment variables for Git auth:

```bash
# Old way (insecure, on agents)
GIT_USERNAME=user
GIT_PASSWORD=token

# New way (secure, on master only)
vcdeploy creds add \
  --name migrated-creds \
  --type basic \
  --url-pattern "*" \
  --username "$GIT_USERNAME"
# Enter password when prompted
```

### From SSH Config

If you used SSH config on agents:

```bash
# Import existing key
vcdeploy ssh-keys import \
  --name legacy-deploy \
  --file ~/.ssh/deploy_key

# Create credential
vcdeploy creds add \
  --name legacy-ssh \
  --type ssh \
  --url-pattern "git@github.com:*" \
  --ssh-key-id 1

# Remove key from agents (no longer needed)
```

## See Also

- [Security Guide](security.md) - How credentials are protected
- [CLI Reference](cli/) - Full credential CLI documentation
- [API Reference](api/) - Credential API endpoints
