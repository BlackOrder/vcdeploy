# CLI Reference

This document provides a comprehensive reference for the `vcdeploy` and `vcdeploy-agent` command-line interfaces.

## Overview

VCDeploy provides two CLI binaries:

- **`vcdeploy`** - Master server CLI for deployment management, configuration, and administration
- **`vcdeploy-agent`** - Agent CLI that runs on target servers and executes deployments

## Global Options

### vcdeploy (Master CLI)

All commands support the following global flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to master config file | `/etc/vcdeploy/master.yaml` |
| `--master` | Master server address for remote CLI | - |
| `--token` | API token for remote CLI authentication | - |
| `--offline` | Force offline/direct mode even if server is running | `false` |

### vcdeploy-agent

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to agent config file | `/etc/vcdeploy/agent.yaml` |

## Commands Overview

### vcdeploy (Master CLI)

```
vcdeploy
├── admin           # Administrator password reset
├── agent           # Agent management
│   ├── list        # List all agents
│   ├── show        # Show agent details
│   ├── delete      # Delete an agent
│   ├── token       # Regenerate agent token
│   └── update      # Update agent binary
├── apikey          # API key management
│   ├── list        # List API keys
│   ├── create      # Create an API key
│   └── revoke      # Revoke an API key
├── audit           # Audit log commands
│   ├── list        # List audit log entries
│   └── export      # Export audit logs
├── certs           # Certificate management
│   ├── list        # List agent certificates
│   ├── show        # Show certificate details
│   ├── revoke      # Revoke a certificate
│   ├── audit       # Certificate audit log
│   └── ca          # CA management
│       ├── list    # List CA certificates
│       └── rotate  # Rotate CA certificate
├── completion      # Generate shell completions
│   ├── bash        # Bash completion
│   ├── zsh         # Zsh completion
│   ├── fish        # Fish completion
│   └── powershell  # PowerShell completion
├── config          # Configuration management
│   ├── show        # Show current config
│   ├── export      # Export config to stdout
│   ├── import      # Import config from file
│   └── set         # Set a config value
├── creds           # Source credential management
│   ├── list        # List credentials
│   ├── add         # Add a credential
│   ├── delete      # Delete a credential
│   └── test        # Test credential connectivity
├── deploy          # Deployment management
│   ├── list        # List deployments
│   ├── status      # Show deployment status
│   ├── cancel      # Cancel a deployment
│   ├── logs        # Show deployment logs
│   └── trigger     # Trigger a deployment
├── master          # Master server management
│   ├── start       # Start the master server
│   ├── stop        # Stop the master server
│   ├── status      # Show server status
│   ├── rotate-key  # Rotate encryption key
│   └── backup      # Backup management
│       ├── create  # Create a backup
│       ├── list    # List backups
│       └── restore # Restore from backup
├── project         # Project management
│   ├── list        # List all projects
│   ├── add         # Add a new project
│   ├── edit        # Edit a project
│   ├── delete      # Delete a project
│   ├── validate    # Validate project config
│   ├── deploy      # Deploy a project
│   ├── rollback    # Rollback to previous release
│   └── health-check # Run health check
├── provision       # Agent provisioning via SSH
│   ├── list        # List provisioning jobs
│   ├── status      # Check job status
│   └── logs        # View provisioning logs
├── recipes         # Recipe and playbook management
│   └── import-yaml # Import YAML config as playbook
├── secret          # Secrets management
│   ├── set         # Set a secret
│   ├── list        # List secrets
│   ├── delete      # Delete a secret
│   ├── import      # Import secrets from .env
│   ├── backup      # Backup all secrets
│   └── restore     # Restore from backup
├── settings        # Settings management
│   ├── list        # List settings
│   ├── get         # Get a setting
│   └── set         # Set a setting
├── show            # Show detailed information
│   ├── project     # Show project details
│   ├── agent       # Show agent details
│   └── deployment  # Show deployment details
├── ssh-keys        # SSH key management
│   ├── list        # List SSH keys
│   ├── generate    # Generate new key
│   ├── import      # Import existing key
│   ├── public      # Show public key
│   └── delete      # Delete a key
├── type            # Project type management
│   ├── list        # List project types
│   ├── create      # Create a project type
│   ├── edit        # Edit a project type
│   └── delete      # Delete a project type
├── totp            # Two-factor authentication management
│   ├── list        # List users with TOTP enabled
│   ├── status      # Show TOTP status for a user
│   └── disable     # Disable TOTP for a user (admin)
├── user            # User management
│   ├── list        # List all users
│   ├── create      # Create a new user
│   ├── delete      # Delete a user
│   └── passwd      # Change user password
├── webhook         # Webhook configuration management
│   ├── add         # Add webhook to a project
│   ├── list        # List webhooks for a project
│   ├── delete      # Delete a webhook
│   ├── test        # Test a webhook (requires server)
│   └── rotate-secret # Rotate webhook secret
└── version         # Print version info
```

### vcdeploy-agent

```
vcdeploy-agent
├── start           # Start the agent
├── status          # Show agent status
├── register        # Register with master
└── version         # Print version info
```

---

## Detailed Command Reference

### admin

Reset or create the administrator account. Useful for lockout recovery when you can't access the web UI.

```bash
# Local mode (direct database access)
vcdeploy admin --username admin --email admin@example.com

# Remote mode (via API)
vcdeploy admin --master localhost:9000 --token <api-token> --username admin
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `-u, --username` | Admin username | `admin` |
| `-p, --password` | Admin password (prompts if not provided) | - |
| `-e, --email` | Admin email address | `admin@localhost` |

---

### agent

Agent management commands. These are top-level commands, not subcommands of `admin`.

#### agent list

List all registered agents.

```bash
vcdeploy agent list
```

**Output columns:**
- ID, Hostname, Status, Version, Last Seen, Labels

#### agent show

Show detailed information about an agent.

```bash
vcdeploy agent show <agent-id>
```

#### agent delete

Remove an agent from the system.

```bash
vcdeploy agent delete <agent-id>
```

#### agent token

Regenerate an agent's authentication token.

```bash
vcdeploy agent token <agent-id>
```

#### agent update

Update agent binary on connected agents.

```bash
# Update a single agent
vcdeploy agent update <agent-id>

# Update all agents
vcdeploy agent update --all

# Update to specific version
vcdeploy agent update <agent-id> --version 1.2.0
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--all` | Update all connected agents |
| `--version` | Specific version to update to |

---

### apikey

API key management for programmatic access.

#### apikey list

List all API keys.

```bash
vcdeploy apikey list
```

#### apikey create

Create a new API key.

```bash
vcdeploy apikey create <name> [flags]
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--expires` | Days until expiry (0 = never) | `0` |

**Example:**
```bash
# Create a key that expires in 30 days
vcdeploy apikey create ci-deploy --expires 30
```

#### apikey revoke

Revoke an API key.

```bash
vcdeploy apikey revoke <key-id>
```

---

### audit

Audit log commands for compliance and troubleshooting.

#### audit list

List audit log entries with optional filters.

```bash
vcdeploy audit list [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--user` | Filter by username |
| `--action` | Filter by action (create, update, delete, deploy, etc.) |
| `--resource` | Filter by resource type (project, user, agent, etc.) |
| `--since` | Show entries since timestamp |
| `--limit` | Maximum entries to return (default: 100) |

**Example:**
```bash
# List deployment actions from the last 24 hours
vcdeploy audit list --action deploy --since 24h
```

#### audit export

Export audit logs to a file.

```bash
vcdeploy audit export [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--format` | Output format: json, csv (default: json) |
| `-o, --output` | Output file path |

---

### completion

Generate shell completion scripts.

```bash
# Bash
vcdeploy completion bash > /etc/bash_completion.d/vcdeploy

# Zsh
vcdeploy completion zsh > "${fpath[1]}/_vcdeploy"

# Fish
vcdeploy completion fish > ~/.config/fish/completions/vcdeploy.fish

# PowerShell
vcdeploy completion powershell > vcdeploy.ps1
```

---

### certs

Certificate management for agent TLS authentication.

#### certs list

List all agent certificates.

```bash
vcdeploy certs list [flags]
```

**Example:**
```bash
vcdeploy certs list --master localhost:9000 --token <token>
```

#### certs show

Show certificate details for an agent.

```bash
vcdeploy certs show <agent-id> [flags]
```

**Example:**
```bash
vcdeploy certs show agent-001 --master localhost:9000 --token <token>
```

#### certs revoke

Revoke an agent's certificate.

```bash
vcdeploy certs revoke <agent-id> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-r, --reason` | Reason for revocation |

**Example:**
```bash
vcdeploy certs revoke agent-001 --reason "Agent compromised" --master localhost:9000 --token <token>
```

#### certs audit

View certificate audit log.

```bash
vcdeploy certs audit [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--limit` | Maximum entries to return (default: 100) |

#### certs ca list

List CA certificates.

```bash
vcdeploy certs ca list [flags]
```

#### certs ca rotate

Rotate the CA certificate.

```bash
vcdeploy certs ca rotate [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-d, --validity-days` | CA validity period in days (default: 365) |

---

### config

Configuration management commands.

#### config show

Display the current configuration.

```bash
vcdeploy config show
```

#### config export

Export configuration to stdout.

```bash
vcdeploy config export > config-backup.yaml
```

#### config import

Import configuration from a file.

```bash
vcdeploy config import <file>
```

#### config set

Set a configuration value.

```bash
vcdeploy config set <key>=<value>
```

**Example:**
```bash
vcdeploy config set server.port=9001
```

---

### deploy

Deployment management commands (view and manage active deployments).

#### deploy list

List recent deployments.

```bash
vcdeploy deploy list [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--project` | Filter by project name |
| `--status` | Filter by status (pending, running, success, failed) |
| `--limit` | Maximum entries (default: 20) |

#### deploy status

Show detailed status of a deployment.

```bash
vcdeploy deploy status <deployment-id>
```

#### deploy cancel

Cancel a running or pending deployment.

```bash
vcdeploy deploy cancel <deployment-id>
```

#### deploy logs

Stream or show deployment logs.

```bash
vcdeploy deploy logs <deployment-id> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-f, --follow` | Follow log output |
| `--tail` | Number of lines to show from end |

#### deploy trigger

Manually trigger a deployment.

```bash
vcdeploy deploy trigger <project-name> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--target` | Specific target to deploy |
| `--branch` | Branch to deploy (overrides project default) |
| `--scheduled` | Schedule deployment for later (RFC3339 format) |

---

### creds

Source credential management for Git repository authentication.

#### creds list

List all configured source credentials.

```bash
vcdeploy creds list [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-t, --type` | Filter by credential type (basic, ssh, token, deploy-key) |

**Example:**
```bash
vcdeploy creds list --master localhost:9000 --token <token>
```

#### creds add

Add a new source credential.

```bash
vcdeploy creds add [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-n, --name` | Credential name (required) |
| `-t, --type` | Credential type: basic, ssh, token, deploy-key (required) |
| `-u, --url-pattern` | URL pattern to match (e.g., github.com/*) |
| `--username` | Username for basic auth |
| `--password` | Password for basic auth (prompts if not provided) |
| `--ssh-key-id` | SSH key ID for ssh type |
| `--stdin` | Read secret value from stdin |

**Examples:**
```bash
# Add a GitHub personal access token
vcdeploy creds add --name github-token --type token --url-pattern "github.com/*" \
  --master localhost:9000 --token <token>

# Add basic auth credentials
vcdeploy creds add --name gitlab-creds --type basic --url-pattern "gitlab.com/myorg/*" \
  --username myuser --master localhost:9000 --token <token>

# Add SSH key reference
vcdeploy creds add --name ssh-deploy --type ssh --url-pattern "git@github.com:myorg/*" \
  --ssh-key-id 123 --master localhost:9000 --token <token>
```

#### creds delete

Delete a source credential.

```bash
vcdeploy creds delete <credential-id> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-f, --force` | Skip confirmation |

#### creds test

Test credential connectivity against a repository.

```bash
vcdeploy creds test <credential-id> <repo-url> [flags]
```

**Example:**
```bash
vcdeploy creds test 123 https://github.com/myorg/myrepo.git \
  --master localhost:9000 --token <token>
```

---

### master

Master server management commands.

#### master start

Start the master server in the foreground.

```bash
vcdeploy master start
```

For production, use systemd:
```bash
sudo systemctl start vcdeploy-master
```

#### master stop

Stop a running master server.

```bash
vcdeploy master stop
```

#### master status

Show master server status.

```bash
vcdeploy master status
```

**Output includes:**
- Server state (running/stopped)
- PID and uptime
- HTTP/gRPC addresses
- Connected agent count
- Database size

#### master rotate-key

Rotate the master encryption key. Re-encrypts all secrets with a new key.

```bash
vcdeploy master rotate-key
```

> **Warning:** Create a backup before rotating keys.

#### master backup

Database backup management.

```bash
# Create a backup
vcdeploy master backup create

# List available backups
vcdeploy master backup list

# Restore from backup
vcdeploy master backup restore <backup-file>
```

---

### project

Project management commands.

#### project list

List all configured projects.

```bash
vcdeploy project list
```

#### project add

Add a new project interactively.

```bash
vcdeploy project add <name>
```

You'll be prompted for:
- Repository URL
- Branch
- Deploy path
- Project type

#### project edit

Edit an existing project.

```bash
vcdeploy project edit <name> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--repo` | Set repository URL |
| `--branch` | Set default branch |
| `--path` | Set deploy path |
| `--type` | Set project type |

#### project delete

Delete a project.

```bash
vcdeploy project delete <name>
```

#### project validate

Validate project configuration without deploying.

```bash
vcdeploy project validate <name>
```

#### project deploy

Deploy a project.

```bash
vcdeploy project deploy <name> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--target` | Target to deploy to (deploys to all if not specified) |
| `--dry-run` | Validate without actually deploying |
| `--force` | Force deploy (bypass deployment lock) |

#### project rollback

Rollback to a previous release.

```bash
vcdeploy project rollback <name> [flags]
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--target` | Target to rollback | all targets |
| `--release` | Specific release number | `0` (previous) |

#### project health-check

Run health check for a project.

```bash
vcdeploy project health-check <name> [flags]
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--url` | Override health check URL | from config |
| `--timeout` | Health check timeout (seconds) | `30` |

---

### provision

Agent provisioning via SSH. Install and configure agents on remote servers.

```bash
vcdeploy provision --host <hostname> [flags]
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--host` | Target host to provision (required) | - |
| `-p, --port` | SSH port | `22` |
| `-u, --user` | SSH user | `root` |
| `--ssh-key` | SSH key ID for authentication | - |
| `--agent-id` | Agent ID to assign | auto-generated |
| `--agent-name` | Agent display name | - |
| `--groups` | Comma-separated groups to assign | - |
| `--no-start` | Don't start agent after installation | - |

**Example:**
```bash
vcdeploy provision --host 192.168.1.100 --user root --ssh-key deploy-key \
  --agent-name "Production Server 1" --groups production,web \
  --master localhost:9000 --token <token>
```

#### provision list

List provisioning jobs.

```bash
vcdeploy provision list [flags]
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `-n, --limit` | Maximum jobs to show | `20` |
| `--status` | Filter by status (pending, running, completed, failed) | - |

#### provision status

Check provisioning job status.

```bash
vcdeploy provision status <job-id> [flags]
```

#### provision logs

View provisioning job logs.

```bash
vcdeploy provision logs <job-id> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-f, --follow` | Follow log output |

---

### recipes

Recipe and playbook management for deployment workflows.

#### recipes import-yaml

Import a project YAML configuration as a recipe playbook.

```bash
vcdeploy recipes import-yaml <yaml-file> [flags]
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--project-id` | Project ID to associate playbook with | - |
| `--name` | Playbook name | `<project>-playbook` |
| `--version` | Playbook version | `v1.0.0` |
| `--create-components` | Create components from hooks | `true` |
| `--activate` | Activate playbook after creation | `false` |
| `--preview` | Preview migration without making changes | `false` |
| `--json` | Output results as JSON | `false` |

**Examples:**
```bash
# Preview migration
vcdeploy recipes import-yaml configs/myproject.yaml --preview

# Import and activate
vcdeploy recipes import-yaml configs/myproject.yaml --project-id 123 --activate

# Import with custom name and version
vcdeploy recipes import-yaml configs/myproject.yaml --name "production-playbook" \
  --version "v2.0.0" --project-id 123
```

---

### secret

Secrets management for deployment-time environment variables.

#### secret set

Set a secret value.

```bash
# Interactive (prompts for value)
vcdeploy secret set <project/scope> <key>

# From stdin
echo "my-secret" | vcdeploy secret set <project/scope> <key> --stdin
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--stdin` | Read value from stdin |

**Examples:**
```bash
# Set database password for "webapp" project
vcdeploy secret set webapp DB_PASSWORD

# Set from stdin
echo "secret123" | vcdeploy secret set webapp API_KEY --stdin
```

#### secret list

List secrets for a project (keys only, values hidden).

```bash
vcdeploy secret list <project>
```

#### secret delete

Delete a secret.

```bash
vcdeploy secret delete <project/scope> <key>
```

#### secret import

Import secrets from .env format via stdin.

```bash
cat .env | vcdeploy secret import <project/scope>
```

#### secret backup

Backup all secrets with passphrase protection.

```bash
vcdeploy secret backup [-o <output-file>]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-o, --output` | Output file path (default: stdout) |

#### secret restore

Restore secrets from backup.

```bash
vcdeploy secret restore <backup-file>
```

---

### settings

Server settings management.

#### settings list

List settings in a category.

```bash
vcdeploy settings list <category>
```

**Categories:**
- `appearance` - UI appearance settings
- `security` - Security settings
- `notifications` - Notification settings
- `server` - Server settings
- `logs` - Logging settings

#### settings get

Get a specific setting.

```bash
vcdeploy settings get <category> <key>
```

#### settings set

Set a setting value.

```bash
vcdeploy settings set <category> <key> <value>
```

---

### ssh-keys

SSH key management for provisioning and Git authentication.

#### ssh-keys list

List all managed SSH keys.

```bash
vcdeploy ssh-keys list [flags]
```

**Example:**
```bash
vcdeploy ssh-keys list --master localhost:9000 --token <token>
```

#### ssh-keys generate

Generate a new Ed25519 SSH key pair.

```bash
vcdeploy ssh-keys generate [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-n, --name` | Key name (required) |
| `-c, --comment` | Key comment (appears in public key) |

**Example:**
```bash
vcdeploy ssh-keys generate --name deploy-key --comment "Deployment key" \
  --master localhost:9000 --token <token>
```

#### ssh-keys import

Import an existing SSH private key.

```bash
vcdeploy ssh-keys import [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-n, --name` | Key name (required) |
| `-f, --file` | Path to private key file |
| `--stdin` | Read private key from stdin |
| `-p, --passphrase` | Passphrase for encrypted key |

**Examples:**
```bash
# Import from file
vcdeploy ssh-keys import --name my-key --file ~/.ssh/id_ed25519 \
  --master localhost:9000 --token <token>

# Import from stdin
cat ~/.ssh/id_ed25519 | vcdeploy ssh-keys import --name my-key --stdin \
  --master localhost:9000 --token <token>
```

#### ssh-keys public

Output the public key in OpenSSH format.

```bash
vcdeploy ssh-keys public <key-id> [flags]
```

**Example:**
```bash
vcdeploy ssh-keys public 123 --master localhost:9000 --token <token> > deploy.pub
```

#### ssh-keys delete

Delete an SSH key.

```bash
vcdeploy ssh-keys delete <key-id> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-f, --force` | Skip confirmation |

---

### show

Show detailed information about resources.

#### show project

Show detailed project information.

```bash
vcdeploy show project <name>
```

#### show agent

Show detailed agent information.

```bash
vcdeploy show agent <agent-id>
```

#### show deployment

Show detailed deployment information.

```bash
vcdeploy show deployment <deployment-id>
```

---

### type

Project type (template) management.

#### type list

List all project types.

```bash
vcdeploy type list
```

#### type create

Create a new project type.

```bash
vcdeploy type create <name>
```

#### type edit

Edit a project type.

```bash
vcdeploy type edit <name> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--description` | Set type description |
| `--build-cmd` | Set build command |

#### type delete

Delete a project type.

```bash
vcdeploy type delete <name>
```

---

### totp

Administrative commands for managing user TOTP two-factor authentication. These commands are for account recovery when users lose access to their authenticator and recovery codes.

#### totp list

List all users who have TOTP enabled.

```bash
vcdeploy totp list
```

**Output:**
```
ID    USERNAME    EMAIL                TOTP ENABLED
1     admin       admin@example.com    true
3     john        john@example.com     true

Total: 2 users with TOTP enabled
```

#### totp status

Show TOTP status for a specific user.

```bash
vcdeploy totp status <username>
```

**Output:**
```
User: john
TOTP Enabled: true
Recovery Codes Remaining: 6
```

#### totp disable

Disable TOTP for a user who has lost access to their authenticator and recovery codes.

```bash
vcdeploy totp disable --user <username> --reason <reason> --confirm
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--user` | Username or user ID (required) |
| `--reason` | Reason for disabling TOTP, for audit (required, min 10 chars) |
| `--confirm` | Confirm this destructive action (required) |

**Example:**
```bash
vcdeploy totp disable --user john --reason "Lost phone, identity verified via video call" --confirm
```

**Security Notes:**
- Always verify user identity through out-of-band means before disabling 2FA
- All actions are logged for audit compliance
- Users will need to re-enable TOTP after logging in

---

### user

User management commands.

#### user list

List all users.

```bash
vcdeploy user list
```

#### user create

Create a new user.

```bash
vcdeploy user create <username> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-e, --email` | User email address |
| `-r, --role` | User role: admin, deployer, viewer |
| `-p, --password` | User password (prompts if not provided) |

#### user delete

Delete a user.

```bash
vcdeploy user delete <username>
```

#### user passwd

Change a user's password.

```bash
vcdeploy user passwd <username>
```

---

### version

Print version information.

```bash
vcdeploy version
```

**Output:**
```
vcdeploy v1.0.0
  commit:     abc1234
  build time: 2024-01-01T00:00:00Z
```

---

### webhook

Manage webhook configurations for projects. Webhooks allow Git providers (GitHub, GitLab, Bitbucket) to trigger deployments automatically.

#### webhook add

Add a webhook configuration to a project.

```bash
vcdeploy webhook add <project> <provider> [flags]
```

**Arguments:**
| Argument | Description |
|----------|-------------|
| `project` | Project name |
| `provider` | Git provider: `github`, `gitlab`, or `bitbucket` |

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--secret` | Webhook secret | Auto-generated |
| `--require-secret` | Require secret validation | `true` |

**Example:**
```bash
# Add GitHub webhook with auto-generated secret
vcdeploy webhook add myproject github

# Add GitLab webhook with custom secret
vcdeploy webhook add myproject gitlab --secret "my-webhook-secret"
```

#### webhook list

List all webhooks configured for a project.

```bash
vcdeploy webhook list <project>
```

**Example:**
```bash
vcdeploy webhook list myproject
```

**Output:**
```
PROVIDER    ENABLED  REQUIRE SECRET
github      true     true
gitlab      true     true
```

#### webhook delete

Remove a webhook configuration from a project.

```bash
vcdeploy webhook delete <project> <provider>
```

**Example:**
```bash
vcdeploy webhook delete myproject github
```

#### webhook test

Send a test payload to verify webhook configuration. This command requires the server to be running.

```bash
vcdeploy webhook test <project> <provider>
```

**Example:**
```bash
vcdeploy webhook test myproject github
```

> **Note:** This command requires a running server and cannot be used in offline mode.

#### webhook rotate-secret

Generate a new secret for a webhook. You must update the secret in your Git provider's settings after running this command.

```bash
vcdeploy webhook rotate-secret <project> <provider>
```

**Example:**
```bash
vcdeploy webhook rotate-secret myproject github
```

**Output:**
```
✓ Webhook secret rotated for myproject/github
New secret: <new-secret>

⚠️  Update this secret in your Git provider's webhook settings.
```

---

## vcdeploy-agent Commands

The agent CLI runs on target servers.

### start

Start the agent and connect to master.

```bash
vcdeploy-agent start [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--master` | Master address (overrides config) |
| `--token` | Registration token (for first connection) |

### status

Show agent status.

```bash
vcdeploy-agent status
```

### register

Register agent with master using a token.

```bash
vcdeploy-agent register --master <address> --token <token>
```

**Required Flags:**
| Flag | Description |
|------|-------------|
| `--master` | Master server address |
| `--token` | Registration token from master |

### version

Print agent version.

```bash
vcdeploy-agent version
```

---

## Environment Variables

| Variable | Equivalent Flag | Description |
|----------|-----------------|-------------|
| `VCDEPLOY_MASTER` | `--master` | Master server address |
| `VCDEPLOY_TOKEN` | `--token` | API authentication token |
| `VCDEPLOY_CONFIG` | `--config` | Config file path |

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 3 | Configuration error |
| 4 | Network/connection error |
| 5 | Authentication error |

---

## Examples

### Initial Setup

```bash
# Start the master server
vcdeploy master start

# Create admin account (prompts for password)
vcdeploy admin --username admin --email admin@example.com

# Add a project
vcdeploy project add myapp

# Set secrets
vcdeploy secret set myapp DB_PASSWORD
echo "key123" | vcdeploy secret set myapp API_KEY --stdin
```

### Deployment Workflow

```bash
# List projects
vcdeploy project list

# Validate before deploying
vcdeploy project validate myapp

# Deploy to production
vcdeploy project deploy myapp --target production

# If something goes wrong, rollback
vcdeploy project rollback myapp --target production

# Check deployment status
vcdeploy deploy status 12345

# View deployment logs
vcdeploy deploy logs 12345 --follow
```

### Remote Administration

```bash
# Set environment variables for remote access
export VCDEPLOY_MASTER=https://deploy.example.com:9000
export VCDEPLOY_TOKEN=vcdeploy_abc123...

# Now all commands go through the API
vcdeploy user list
vcdeploy deploy list
vcdeploy agent list
```

### Backup and Recovery

```bash
# Create database backup
vcdeploy master backup create

# List backups
vcdeploy master backup list

# Backup secrets separately
vcdeploy secret backup -o secrets.enc

# Restore (stop server first)
vcdeploy master stop
vcdeploy master backup restore backup-2024-01-01.db
vcdeploy secret restore secrets.enc
vcdeploy master start
```

### Agent Setup

```bash
# On the master: generate a registration token
vcdeploy agent token new-agent

# On the target server: register and start the agent
vcdeploy-agent register --master master.example.com:9001 --token <token>
vcdeploy-agent start
```

---

## API-Only Features

Some features are intentionally available only via the REST API or Web UI, not the CLI. This is by design.

| API Endpoint | Feature | Reason |
|--------------|---------|--------|
| `/api/v1/blocked-ips` | IP blocking/unblocking | Security feature best managed via UI for audit visibility |
| `/api/v1/host-keys` | SSH host key management | Low-frequency admin operation |
| `/api/v1/jump-servers` | Jump server configuration | Low-frequency admin operation |
| `/api/v1/binaries` | Agent binary management | Internal system feature |
| `/api/v1/tls/*` | TLS certificate management | Server configuration via UI |
| `/api/v1/recipes/*` | Full recipe CRUD | Use `recipes import-yaml` for CLI workflow |

If you need CLI access to these features, use `curl` or similar tools with an API key:

```bash
# Example: List blocked IPs
curl -H "Authorization: Bearer $VCDEPLOY_TOKEN" \
     https://vcdeploy.example.com/api/v1/blocked-ips

# Example: Block an IP
curl -X POST -H "Authorization: Bearer $VCDEPLOY_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"ip":"192.168.1.100","reason":"suspicious activity"}' \
     https://vcdeploy.example.com/api/v1/blocked-ips
```

---

## Command Conventions

### Resource Lifecycle Verbs

| Verb | Usage | Example |
|------|-------|---------|
| `create` | Create a new resource | `vcdeploy user create` |
| `list` | List all resources | `vcdeploy project list` |
| `show` | Show resource details | `vcdeploy agent show` |
| `edit` | Modify an existing resource | `vcdeploy project edit` |
| `delete` | Permanently remove a resource | `vcdeploy project delete` |

### Special Verbs

| Verb | Usage | When to Use |
|------|-------|-------------|
| `revoke` | Invalidate without deleting | `apikey revoke`, `certs revoke` - preserves audit trail |
| `trigger` | Start an operation | `deploy trigger` |
| `cancel` | Cancel a running operation | `deploy cancel` |
| `rotate` | Regenerate/replace credentials | `webhook rotate-secret`, `certs ca rotate` |

**Why `revoke` instead of `delete`?**

For credentials like API keys and certificates, we use `revoke` because:
1. The resource's existence must be remembered for audit purposes
2. The credential should no longer grant access
3. Deletion would lose the audit trail of what was revoked and when

---

## See Also

- [Installation Guide](../installation.md)
- [Master Configuration](../config/master.md)
- [Agent Configuration](../config/agent.md)
- [API Reference](../api.md)
