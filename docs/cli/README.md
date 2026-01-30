# CLI Reference

This document provides a comprehensive reference for the `vcdeploy` command-line interface.

## Global Options

All commands support the following global flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to config file | `/etc/vcdeploy/master.yaml` |
| `--master` | Master server address for remote CLI | - |
| `--token` | API token for remote CLI authentication | - |

## Commands Overview

```
vcdeploy
├── admin       # Administrator account management
├── master      # Master server management
│   ├── start   # Start the master server
│   ├── stop    # Stop the master server
│   ├── status  # Show server status
│   ├── rotate-key  # Rotate encryption key
│   └── backup  # Backup management
│       ├── create  # Create a backup
│       ├── list    # List backups
│       └── restore # Restore from backup
├── project     # Project management
│   ├── list    # List all projects
│   ├── add     # Add a new project
│   ├── edit    # Edit a project
│   ├── delete  # Delete a project
│   ├── validate    # Validate project config
│   ├── deploy      # Deploy a project
│   ├── rollback    # Rollback to previous release
│   └── health-check # Run health check
├── type        # Project type management
│   ├── list    # List project types
│   ├── create  # Create a project type
│   ├── edit    # Edit a project type
│   └── delete  # Delete a project type
├── secret      # Secrets management
│   ├── set     # Set a secret
│   ├── list    # List secrets
│   ├── delete  # Delete a secret
│   ├── import  # Import secrets from .env
│   ├── backup  # Backup all secrets
│   └── restore # Restore from backup
├── settings    # Settings management
│   ├── list    # List settings
│   ├── get     # Get a setting
│   └── set     # Set a setting
└── version     # Print version info
```

## Detailed Command Reference

### admin

Manage the administrator account. Supports both local (direct database) and remote (API) modes.

```bash
# Local mode (lockout recovery - direct database access)
vcdeploy admin --username admin --email admin@example.com

# Remote mode (requires API token)
vcdeploy admin --master localhost:9000 --token <api-token> --username admin
```

**Flags:**
- `-u, --username` - Admin username (default: "admin")
- `-p, --password` - Admin password (prompts if not provided)
- `-e, --email` - Admin email address (default: "admin@localhost")

**Subcommands:**
- `admin user list` - List all users
- `admin user create <username>` - Create a new user
- `admin user delete <username>` - Delete a user
- `admin user passwd <username>` - Change user password
- `admin agent list` - List all agents
- `admin agent show <id>` - Show agent details
- `admin agent delete <id>` - Delete an agent
- `admin agent token <id>` - Regenerate agent token
- `admin deployment list` - List deployments
- `admin deployment status <id>` - Show deployment status
- `admin deployment cancel <id>` - Cancel a deployment
- `admin deployment logs <id>` - Show deployment logs
- `admin deployment trigger <project>` - Trigger deployment
- `admin config show` - Show current config
- `admin config export` - Export config to stdout
- `admin config import <file>` - Import config from file
- `admin config set <key>=<value>` - Set a config value
- `admin apikey list` - List API keys
- `admin apikey create <name>` - Create an API key
- `admin apikey revoke <id>` - Revoke an API key

### master

Master server management commands.

#### master start

Start the master server.

```bash
vcdeploy master start
```

The server runs in the foreground. Use systemd or supervisor for production deployment.

#### master stop

Stop a running master server.

```bash
vcdeploy master stop
```

#### master status

Show the current status of the master server.

```bash
vcdeploy master status
```

Output includes:
- Server state (running/stopped)
- PID and uptime
- HTTP/gRPC addresses
- Agent connection count

#### master rotate-key

Rotate the master encryption key. This re-encrypts all secrets with a new key.

```bash
vcdeploy master rotate-key
```

**Warning:** Ensure you have a backup before rotating keys.

#### master backup

Database backup management.

```bash
# Create a backup
vcdeploy master backup create

# List available backups
vcdeploy master backup list

# Restore from a backup
vcdeploy master backup restore <backup-file>
```

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
- `--repo` - Set repository URL
- `--branch` - Set default branch
- `--path` - Set deploy path
- `--type` - Set project type

#### project delete

Delete a project.

```bash
vcdeploy project delete <name>
```

#### project validate

Validate a project configuration without deploying.

```bash
vcdeploy project validate <name>
```

#### project deploy

Deploy a project.

```bash
vcdeploy project deploy <name> [flags]
```

**Flags:**
- `--target` - Target to deploy to (if not specified, deploys to all)
- `--dry-run` - Validate without actually deploying
- `--force` - Force deploy (bypass deployment lock)

#### project rollback

Rollback to a previous release.

```bash
vcdeploy project rollback <name> [flags]
```

**Flags:**
- `--target` - Target to rollback
- `--release` - Specific release number (default: previous release)

#### project health-check

Run a health check for a project.

```bash
vcdeploy project health-check <name> [flags]
```

**Flags:**
- `--url` - Override the health check URL
- `--timeout` - Health check timeout in seconds (default: 30)

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
- `--description` - Set type description
- `--build-cmd` - Set build command

#### type delete

Delete a project type.

```bash
vcdeploy type delete <name>
```

### secret

Secrets management for deployment-time environment variables.

#### secret set

Set a secret value.

```bash
# Interactive (prompts for value)
vcdeploy secret set <project/scope> <key>

# From stdin
echo "my-secret-value" | vcdeploy secret set <project/scope> <key> --stdin
```

**Examples:**
```bash
# Set a database password for project "webapp"
vcdeploy secret set webapp DB_PASSWORD

# Set from stdin
echo "secret123" | vcdeploy secret set webapp API_KEY --stdin
```

#### secret list

List secrets for a project (keys only, values are hidden).

```bash
vcdeploy secret list <project>
```

#### secret delete

Delete a secret.

```bash
vcdeploy secret delete <project/scope> <key>
```

#### secret import

Import secrets from a .env file via stdin.

```bash
cat .env | vcdeploy secret import <project/scope>
```

#### secret backup

Backup all secrets with passphrase protection.

```bash
vcdeploy secret backup [-o <output-file>]
```

**Flags:**
- `-o, --output` - Output file path (default: stdout)

#### secret restore

Restore secrets from a backup file.

```bash
vcdeploy secret restore <backup-file>
```

### settings

Settings management for server configuration.

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

Get a specific setting value.

```bash
vcdeploy settings get <category> <key>
```

#### settings set

Set a setting value.

```bash
vcdeploy settings set <category> <key> <value>
```

### version

Print version information.

```bash
vcdeploy version
```

Output:
```
vcdeploy v1.0.0
  commit:     abc1234
  build time: 2024-01-01T00:00:00Z
```

## Environment Variables

The following environment variables can be used instead of flags:

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

## Examples

### Initial Setup

```bash
# Start the master server
vcdeploy master start

# Create admin account (will prompt for password)
vcdeploy admin --username admin --email admin@example.com

# Add a project
vcdeploy project add myapp

# Set secrets
vcdeploy secret set myapp DB_PASSWORD
vcdeploy secret set myapp API_KEY --stdin < api-key.txt
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
```

### Remote Administration

```bash
# Set environment variables for remote access
export VCDEPLOY_MASTER=https://deploy.example.com:9000
export VCDEPLOY_TOKEN=vcdeploy_abc123...

# Now all commands go through the API
vcdeploy admin user list
vcdeploy admin deployment list
vcdeploy admin agent list
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

## See Also

- [Installation Guide](../installation.md)
- [Configuration Reference](../config/master.md)
- [API Reference](../api.md)
