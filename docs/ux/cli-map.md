# vcdeploy CLI Map

> **Version:** 1.0  
> **Binary:** `vcdeploy`  
> **Shell Completions:** bash, zsh, fish, powershell

This document defines the canonical CLI command structure with comprehensive help text and examples.

---

## Table of Contents

1. [Design Principles](#design-principles)
2. [Global Flags](#global-flags)
3. [Command Reference](#command-reference)
4. [Help Text Standards](#help-text-standards)

---

## Design Principles

### Noun-Verb Structure

Commands follow `vcdeploy <resource> <action>` pattern:

```bash
vcdeploy project create      # Good: noun-verb
vcdeploy create-project      # Bad: verb-noun
```

### Consistent Verbs

| Verb | Meaning | Notes |
|------|---------|-------|
| `list` | List resources | Returns table or JSON array |
| `show` | Show single resource | Returns detailed view |
| `create` | Create resource | Interactive or with flags |
| `update` | Modify resource | Partial updates supported |
| `delete` | Remove resource | Requires confirmation unless `--force` |

**Never use:** `add`, `remove`, `get`, `status`, `info`

### Output Formats

All commands support `--output` / `-o` flag:

| Format | Description |
|--------|-------------|
| `table` | Human-readable table (default) |
| `json` | JSON for scripting |
| `yaml` | YAML for config files |
| `wide` | Extended table with more columns |

---

## Global Flags

```
Global Flags:
  -m, --master string   Master server address (default: from config or localhost:9000)
  -t, --token string    Authentication token (default: from config)
  -o, --output string   Output format: table, json, yaml, wide (default: table)
      --no-color        Disable colored output
  -v, --verbose         Enable verbose logging
  -h, --help            Help for any command
```

### Configuration File

Default locations (in order):
1. `$VCDEPLOY_CONFIG` environment variable
2. `./vcdeploy.yaml`
3. `~/.config/vcdeploy/config.yaml`
4. `/etc/vcdeploy/client.yaml`

```yaml
master: "https://vcdeploy.example.com:9000"
token: "eyJ..."
output: "table"
```

---

## Command Reference

### Authentication

#### login

```
vcdeploy login [flags]

Log in to a vcdeploy master server and store credentials locally.

The token is stored in ~/.config/vcdeploy/config.yaml and used for
subsequent commands. If 2FA is enabled on your account, you will be
prompted for a TOTP code.

Flags:
  -u, --username string   Username for authentication
  -p, --password string   Password (omit for interactive prompt)
      --totp string       TOTP code if 2FA is enabled

Examples:
  # Interactive login (prompts for password)
  vcdeploy login --master vcdeploy.example.com:9000 --username admin

  # Non-interactive login (for scripts)
  vcdeploy login -m vcdeploy.example.com:9000 -u admin -p "$PASSWORD"

  # Login with 2FA
  vcdeploy login -m vcdeploy.example.com:9000 -u admin --totp 123456
```

#### logout

```
vcdeploy logout

Log out and clear stored credentials.

Removes the authentication token from the local configuration file.
Does not invalidate the token on the server.

Examples:
  vcdeploy logout
```

#### whoami

```
vcdeploy whoami [flags]

Display information about the currently authenticated user.

Shows username, role, and session information. Useful for verifying
which account is currently active.

Examples:
  # Show current user
  vcdeploy whoami

  # Show detailed session info
  vcdeploy whoami -o json
```

---

### Dashboard

#### status

```
vcdeploy status [flags]

Display system dashboard with key metrics.

Shows total projects, agents, deployments, and success rate.
Equivalent to the web UI dashboard.

Flags:
      --watch           Continuously update (default: false)
      --interval int    Update interval in seconds (default: 5)

Examples:
  # Show current status
  vcdeploy status

  # Watch status updates
  vcdeploy status --watch --interval 10
```

---

### Projects

#### project

```
vcdeploy project <command> [flags]

Manage deployment projects.

A project defines what to deploy, where to deploy it, and how.
Projects reference a Git repository and specify deployment hooks,
health checks, and target agents.

Available Commands:
  list        List all projects
  show        Show project details
  create      Create a new project
  update      Update project configuration
  delete      Delete a project
  deploy      Trigger a deployment
  rollback    Trigger a rollback
  validate    Validate project configuration
  clone       Clone project to new name
  health      Run health check

Use "vcdeploy project <command> --help" for more information.
```

#### project list

```
vcdeploy project list [flags]

List all configured projects.

Displays project name, type, repository, and deployment status.
Use filters to narrow results.

Flags:
      --type string     Filter by project type
      --agent string    Filter by assigned agent
      --search string   Search by name

Examples:
  # List all projects
  vcdeploy project list

  # Filter by type
  vcdeploy project list --type laravel

  # Search by name
  vcdeploy project list --search api

  # Output as JSON
  vcdeploy project list -o json
```

#### project show

```
vcdeploy project show <name> [flags]

Show detailed information about a project.

Displays full project configuration including repository settings,
deployment hooks, health checks, and assigned agents.

Examples:
  # Show project details
  vcdeploy project show myapp

  # Show as YAML (useful for backup)
  vcdeploy project show myapp -o yaml > myapp.yaml
```

#### project create

```
vcdeploy project create <name> [flags]

Create a new deployment project.

Creates a project with the specified configuration. You can provide
configuration via flags or import from a YAML file.

Flags:
      --type string        Project type (required)
      --repo string        Git repository URL (required)
      --branch string      Default branch (default: main)
      --path string        Deploy path on target (required)
      --agents strings     Comma-separated agent names or groups
  -f, --file string        Import from YAML file

Examples:
  # Create with flags
  vcdeploy project create myapp \
    --type laravel \
    --repo git@github.com:org/myapp.git \
    --branch main \
    --path /var/www/myapp \
    --agents web-1,web-2

  # Create from file
  vcdeploy project create myapp -f myapp.yaml

  # Create interactively (prompts for values)
  vcdeploy project create myapp
```

#### project update

```
vcdeploy project update <name> [flags]

Update an existing project's configuration.

Only specified flags are updated; other settings remain unchanged.

Flags:
      --branch string      Update default branch
      --path string        Update deploy path
      --agents strings     Update assigned agents
      --enabled bool       Enable/disable project
  -f, --file string        Update from YAML file

Examples:
  # Update branch
  vcdeploy project update myapp --branch develop

  # Update agents
  vcdeploy project update myapp --agents web-1,web-2,web-3

  # Update from file
  vcdeploy project update myapp -f myapp-updated.yaml
```

#### project delete

```
vcdeploy project delete <name> [flags]

Delete a project.

Removes the project configuration. Does not affect deployed files
on agents. Active deployments will be cancelled.

Flags:
      --force   Skip confirmation prompt

Examples:
  # Delete with confirmation
  vcdeploy project delete myapp

  # Force delete (no prompt)
  vcdeploy project delete myapp --force
```

#### project deploy

```
vcdeploy project deploy <name> [flags]

Trigger a deployment for a project.

Deploys the specified branch/commit to all configured agents.
Use --dry-run to simulate without making changes.

Flags:
      --branch string      Branch to deploy (default: project default)
      --commit string      Specific commit SHA
      --agents strings     Override target agents
      --dry-run            Simulate deployment only
      --wait               Wait for completion
      --follow             Follow deployment logs

Examples:
  # Deploy default branch
  vcdeploy project deploy myapp

  # Deploy specific branch
  vcdeploy project deploy myapp --branch feature/new-ui

  # Deploy specific commit
  vcdeploy project deploy myapp --commit abc123

  # Dry run
  vcdeploy project deploy myapp --dry-run

  # Deploy and watch logs
  vcdeploy project deploy myapp --follow
```

#### project rollback

```
vcdeploy project rollback <name> [flags]

Trigger a rollback to a previous deployment.

Rolls back to the specified deployment or the last successful one.

Flags:
      --to string          Deployment ID to rollback to
      --wait               Wait for completion
      --follow             Follow rollback logs

Examples:
  # Rollback to last successful deployment
  vcdeploy project rollback myapp

  # Rollback to specific deployment
  vcdeploy project rollback myapp --to deploy-42

  # Rollback and watch logs
  vcdeploy project rollback myapp --follow
```

#### project clone

```
vcdeploy project clone <source> <target> [flags]

Clone a project to a new name.

Creates a copy of the project with a new name. Useful for creating
staging/production variants of the same application.

Flags:
      --branch string   Override branch in clone
      --path string     Override deploy path in clone

Examples:
  # Clone project
  vcdeploy project clone myapp myapp-staging

  # Clone with different branch
  vcdeploy project clone myapp myapp-staging --branch staging
```

---

### Deployments

#### deploy

```
vcdeploy deploy <command> [flags]

Manage deployment records.

View, monitor, and control deployment executions. For triggering
new deployments, use "vcdeploy project deploy".

Available Commands:
  list        List deployment records
  show        Show deployment details
  logs        View deployment logs
  cancel      Cancel a running deployment
  retry       Retry a failed deployment

Use "vcdeploy deploy <command> --help" for more information.
```

#### deploy list

```
vcdeploy deploy list [flags]

List deployment records.

Shows deployment history with status, project, and timing.

Flags:
      --project string   Filter by project
      --status string    Filter by status: pending, running, success, failed, cancelled
      --type string      Filter by type: deploy, rollback
      --limit int        Maximum records to show (default: 20)
      --all              Show all records (no limit)

Examples:
  # List recent deployments
  vcdeploy deploy list

  # Filter by project
  vcdeploy deploy list --project myapp

  # Show only failed deployments
  vcdeploy deploy list --status failed

  # Show all deployments as JSON
  vcdeploy deploy list --all -o json
```

#### deploy show

```
vcdeploy deploy show <id> [flags]

Show detailed deployment information.

Displays full deployment record including timing, logs summary,
and agent results.

Examples:
  # Show deployment
  vcdeploy deploy show deploy-123

  # Show as JSON
  vcdeploy deploy show deploy-123 -o json
```

#### deploy logs

```
vcdeploy deploy logs <id> [flags]

View deployment logs.

Displays logs from the deployment execution. Use --follow for
real-time streaming.

Flags:
      --follow    Stream logs in real-time
      --tail int  Number of lines from end (default: all)

Examples:
  # View all logs
  vcdeploy deploy logs deploy-123

  # Follow logs in real-time
  vcdeploy deploy logs deploy-123 --follow

  # Show last 50 lines
  vcdeploy deploy logs deploy-123 --tail 50
```

#### deploy cancel

```
vcdeploy deploy cancel <id> [flags]

Cancel a running deployment.

Sends cancellation signal to the deployment. The deployment may
take a moment to stop gracefully.

Flags:
      --force   Force immediate cancellation

Examples:
  # Cancel deployment
  vcdeploy deploy cancel deploy-123

  # Force cancel
  vcdeploy deploy cancel deploy-123 --force
```

#### deploy retry

```
vcdeploy deploy retry <id> [flags]

Retry a failed deployment.

Creates a new deployment with the same configuration as the
failed one.

Flags:
      --wait     Wait for completion
      --follow   Follow deployment logs

Examples:
  # Retry deployment
  vcdeploy deploy retry deploy-123

  # Retry and watch logs
  vcdeploy deploy retry deploy-123 --follow
```

---

### Agents

#### agent

```
vcdeploy agent <command> [flags]

Manage deployment agents.

Agents are the target servers where code is deployed. They connect
to the master via gRPC and receive deployment commands.

Available Commands:
  list        List registered agents
  show        Show agent details
  update      Update agent configuration
  delete      Remove an agent
  token       Generate registration token
  provision   Manage agent provisioning

Use "vcdeploy agent <command> --help" for more information.
```

#### agent list

```
vcdeploy agent list [flags]

List registered agents.

Shows agent name, status, groups, and last heartbeat.

Flags:
      --status string   Filter by status: online, offline, all
      --group string    Filter by group name
      --search string   Search by name

Examples:
  # List all agents
  vcdeploy agent list

  # Show only online agents
  vcdeploy agent list --status online

  # Filter by group
  vcdeploy agent list --group web-servers

  # Wide output with more details
  vcdeploy agent list -o wide
```

#### agent show

```
vcdeploy agent show <id> [flags]

Show detailed agent information.

Displays agent configuration, status, groups, tags, and recent
deployment history.

Examples:
  # Show agent
  vcdeploy agent show agent-1

  # Show as JSON
  vcdeploy agent show agent-1 -o json
```

#### agent update

```
vcdeploy agent update <id> [flags]

Update agent configuration.

Modify agent name, groups, or tags. Does not affect the agent
binary or its connection.

Flags:
      --name string      Update agent name
      --groups strings   Update group memberships
      --tags strings     Update tags

Examples:
  # Update name
  vcdeploy agent update agent-1 --name web-server-1

  # Update groups
  vcdeploy agent update agent-1 --groups web-servers,production

  # Update tags
  vcdeploy agent update agent-1 --tags region=us-east,tier=frontend
```

#### agent delete

```
vcdeploy agent delete <id> [flags]

Remove an agent from the master.

Unregisters the agent. The agent process will continue running
but won't receive deployments until re-registered.

Flags:
      --force   Skip confirmation prompt

Examples:
  # Delete with confirmation
  vcdeploy agent delete agent-1

  # Force delete
  vcdeploy agent delete agent-1 --force
```

#### agent token

```
vcdeploy agent token [flags]

Generate a registration token for new agents.

Creates a one-time token for agent registration. The token expires
after the specified duration or first use.

Flags:
      --expires duration   Token expiration (default: 24h)
      --groups strings     Pre-assign groups to registered agent

Examples:
  # Generate token
  vcdeploy agent token

  # Token valid for 1 hour
  vcdeploy agent token --expires 1h

  # Token with pre-assigned groups
  vcdeploy agent token --groups web-servers,production
```

#### agent provision

```
vcdeploy agent provision <command> [flags]

Manage agent provisioning jobs.

Provisioning installs and configures the vcdeploy agent on target
servers via SSH.

Available Commands:
  create      Start a provisioning job
  list        List provisioning jobs
  show        Show provisioning job details
  logs        View provisioning logs
  cancel      Cancel a provisioning job

Examples:
  # Provision a new agent
  vcdeploy agent provision create --host 192.168.1.10 --user deploy

  # List provisioning jobs
  vcdeploy agent provision list

  # Watch provisioning logs
  vcdeploy agent provision logs job-123 --follow
```

---

### Users

#### user

```
vcdeploy user <command> [flags]

Manage user accounts.

Control access to the vcdeploy system by managing user accounts,
passwords, and two-factor authentication.

Available Commands:
  list        List users
  show        Show user details
  create      Create a new user
  update      Update user
  delete      Delete a user
  password    Change user password
  totp        Manage two-factor authentication

Use "vcdeploy user <command> --help" for more information.
```

#### user list

```
vcdeploy user list [flags]

List all user accounts.

Shows username, role, 2FA status, and last login.

Examples:
  # List users
  vcdeploy user list

  # Output as JSON
  vcdeploy user list -o json
```

#### user create

```
vcdeploy user create <username> [flags]

Create a new user account.

Creates a user with the specified role. Password will be prompted
interactively unless provided via flag.

Flags:
      --password string   User password (prompts if not provided)
      --email string      User email address
      --role string       User role: admin, operator, viewer (default: operator)

Examples:
  # Create user interactively
  vcdeploy user create john

  # Create admin user
  vcdeploy user create john --role admin --email john@example.com

  # Create with password (for automation)
  vcdeploy user create john --password "$PASSWORD" --role operator
```

#### user password

```
vcdeploy user password <username> [flags]

Change a user's password.

Changes the password for the specified user. Admins can change
any user's password; users can change their own.

Flags:
      --password string   New password (prompts if not provided)

Examples:
  # Change password interactively
  vcdeploy user password john

  # Change password non-interactively
  vcdeploy user password john --password "$NEW_PASSWORD"
```

#### user totp

```
vcdeploy user totp <command> [username] [flags]

Manage two-factor authentication for users.

If username is omitted, operates on the current user.

Available Commands:
  show        Show TOTP status
  setup       Begin TOTP setup (shows QR code)
  enable      Enable TOTP after setup
  disable     Disable TOTP
  recovery    Regenerate recovery codes

Examples:
  # Check own TOTP status
  vcdeploy user totp show

  # Setup TOTP for yourself
  vcdeploy user totp setup

  # Admin: disable TOTP for a user
  vcdeploy user totp disable john
```

---

### Configuration

#### config

```
vcdeploy config <command> [flags]

Manage server settings.

Configure global server settings organized by category.

Available Commands:
  list        List all settings
  show        Show settings for a category
  get         Get a specific setting
  set         Update a setting
  export      Export settings to file
  import      Import settings from file

Use "vcdeploy config <command> --help" for more information.
```

#### config list

```
vcdeploy config list [flags]

List all server settings grouped by category.

Categories: general, appearance, security, notifications, deployments

Examples:
  # List all settings
  vcdeploy config list

  # Output as JSON
  vcdeploy config list -o json
```

#### config show

```
vcdeploy config show <category> [flags]

Show all settings in a category.

Examples:
  # Show security settings
  vcdeploy config show security

  # Show notifications settings as YAML
  vcdeploy config show notifications -o yaml
```

#### config set

```
vcdeploy config set <category>.<key> <value> [flags]

Update a specific setting.

Use dot notation to specify the setting path.

Examples:
  # Set session timeout
  vcdeploy config set security.session_timeout 24h

  # Enable 2FA requirement
  vcdeploy config set security.require_2fa true

  # Set notification webhook
  vcdeploy config set notifications.webhook.url https://example.com/webhook
```

---

### Secrets

#### secret

```
vcdeploy secret <command> [flags]

Manage deployment secrets.

Secrets are encrypted values injected into deployments. They can
be scoped globally or per-project.

Available Commands:
  list        List secrets (names only)
  show        Show secret value
  set         Create or update a secret
  delete      Delete a secret
  bulk        Bulk import secrets
  export      Export encrypted secrets
  import      Import encrypted secrets

Use "vcdeploy secret <command> --help" for more information.
```

#### secret list

```
vcdeploy secret list [flags]

List secret names (values are not shown).

Flags:
      --project string   Filter by project scope
      --scope string     Filter by scope: global, project

Examples:
  # List all secrets
  vcdeploy secret list

  # List project secrets
  vcdeploy secret list --project myapp

  # List global secrets only
  vcdeploy secret list --scope global
```

#### secret show

```
vcdeploy secret show <scope> <key> [flags]

Show a secret value.

Scope must be "global" or a project name.

Examples:
  # Show global secret
  vcdeploy secret show global DB_PASSWORD

  # Show project secret
  vcdeploy secret show myapp API_KEY
```

#### secret set

```
vcdeploy secret set <scope> <key> [flags]

Create or update a secret.

Value can be provided via flag, stdin, or file. If none provided,
prompts interactively (value hidden).

Flags:
      --value string   Secret value
      --file string    Read value from file
      --stdin          Read value from stdin

Examples:
  # Set interactively (value hidden)
  vcdeploy secret set global DB_PASSWORD

  # Set from flag
  vcdeploy secret set global DB_PASSWORD --value "secret123"

  # Set from file
  vcdeploy secret set global SSH_KEY --file ~/.ssh/id_rsa

  # Set from stdin (useful for pipes)
  echo "secret123" | vcdeploy secret set global DB_PASSWORD --stdin

  # Set project-scoped secret
  vcdeploy secret set myapp API_KEY --value "key123"
```

#### secret bulk

```
vcdeploy secret bulk [flags]

Bulk import secrets from file.

File format (JSON):
{
  "DB_HOST": "localhost",
  "DB_PORT": "5432"
}

Or .env format:
DB_HOST=localhost
DB_PORT=5432

Flags:
      --project string   Project scope (omit for global)
      --file string      File to import (required)
      --format string    File format: json, env (auto-detected)

Examples:
  # Import from .env file
  vcdeploy secret bulk --file .env

  # Import project secrets from JSON
  vcdeploy secret bulk --project myapp --file secrets.json
```

---

### API Keys

#### api-key

```
vcdeploy api-key <command> [flags]

Manage API keys for programmatic access.

API keys can be used instead of bearer tokens for automation and
CI/CD integrations.

Available Commands:
  list        List API keys
  show        Show API key details
  create      Create a new API key
  revoke      Revoke an API key

Use "vcdeploy api-key <command> --help" for more information.
```

#### api-key create

```
vcdeploy api-key create <name> [flags]

Create a new API key.

The key value is shown only once at creation. Store it securely.

Flags:
      --expires int      Days until expiration (0 = never)
      --scopes strings   Allowed scopes (default: all)

Examples:
  # Create key that never expires
  vcdeploy api-key create ci-deploy

  # Create key expiring in 30 days
  vcdeploy api-key create ci-deploy --expires 30

  # Create key with limited scope
  vcdeploy api-key create ci-deploy --scopes deployments:write,projects:read
```

---

### Certificates & TLS

#### cert

```
vcdeploy cert <command> [flags]

Manage certificates and TLS configuration.

Control agent certificates, CA rotation, and HTTPS settings.

Available Commands:
  list        List agent certificates
  show        Show certificate details
  revoke      Revoke an agent certificate
  audit       View certificate audit log
  ca          Manage Certificate Authority
  tls         Manage TLS/HTTPS settings

Use "vcdeploy cert <command> --help" for more information.
```

#### cert ca

```
vcdeploy cert ca <command> [flags]

Manage the internal Certificate Authority.

The CA signs agent certificates for mTLS authentication.

Available Commands:
  show        Show CA certificate info
  rotate      Rotate CA certificate

Examples:
  # Show CA info
  vcdeploy cert ca show

  # Rotate CA (will require agent re-registration)
  vcdeploy cert ca rotate
```

#### cert tls

```
vcdeploy cert tls <command> [flags]

Manage HTTPS/TLS configuration.

Configure TLS mode, certificates, and ACME (Let's Encrypt).

Available Commands:
  show        Show TLS configuration
  update      Update TLS settings
  renew       Force ACME certificate renewal

Examples:
  # Show TLS status
  vcdeploy cert tls show

  # Force Let's Encrypt renewal
  vcdeploy cert tls renew
```

---

### Recipes

#### recipe

```
vcdeploy recipe <command> [flags]

Manage deployment recipes.

Recipes are reusable deployment configurations including components,
playbooks, and activations.

Available Commands:
  list        List playbooks
  show        Show playbook details
  create      Create a playbook
  update      Update a playbook
  delete      Delete a playbook
  component   Manage recipe components
  activation  Manage recipe activations
  approval    Manage raw approvals
  export      Export all recipes
  import      Import recipes

Use "vcdeploy recipe <command> --help" for more information.
```

---

### Other Resources

#### credential

```
vcdeploy credential <command> [flags]

Manage Git credentials for repository access.

Credentials authenticate to Git providers (GitHub, GitLab, etc.)
for cloning private repositories.

Available Commands:
  list        List credentials
  show        Show credential details
  create      Create credential
  update      Update credential
  delete      Delete credential
  test        Test credential against a repository

Aliases: credentials, creds

Examples:
  # Create GitHub token
  vcdeploy credential create --name github --type token

  # Test credential
  vcdeploy credential test github https://github.com/org/repo.git
```

#### ssh-key

```
vcdeploy ssh-key <command> [flags]

Manage SSH keys for repository access and agent provisioning.

Available Commands:
  list        List SSH keys
  show        Show SSH key (public part)
  generate    Generate new SSH key pair
  import      Import existing SSH key
  delete      Delete SSH key
```

#### host-key

```
vcdeploy host-key <command> [flags]

Manage known host keys for SSH verification.

Available Commands:
  list        List known host keys
  show        Show host key details
  create      Add host key manually
  delete      Remove host key
  scan        Scan host and add its key
```

#### jump-server

```
vcdeploy jump-server <command> [flags]

Manage SSH jump servers (bastion hosts).

Available Commands:
  list        List jump servers
  show        Show jump server details
  create      Add jump server
  update      Update jump server
  delete      Remove jump server
  test        Test jump server connectivity
```

#### blocked-ip

```
vcdeploy blocked-ip <command> [flags]

Manage IP blocking for security.

Available Commands:
  list        List blocked IPs
  create      Block an IP address
  delete      Unblock an IP address
```

#### webhook

```
vcdeploy webhook <command> [flags]

Manage project webhooks for auto-deployment.

Available Commands:
  list        List webhooks for a project
  show        Show webhook details
  create      Configure webhook for provider
  delete      Remove webhook
  test        Send test webhook
  rotate      Rotate webhook secret
```

#### health-check

```
vcdeploy health-check <command> [flags]

Manage deployment health checks.

Available Commands:
  list        List health check configurations
  show        Show health check details
  create      Create health check
  update      Update health check
  delete      Delete health check
  run         Run health check manually
  status      Show global health status
```

#### binary

```
vcdeploy binary <command> [flags]

Manage agent binary uploads for provisioning.

Available Commands:
  list        List uploaded binaries
  show        Show binary details
  upload      Upload agent binary
  download    Download agent binary
  delete      Delete agent binary
```

#### audit

```
vcdeploy audit <command> [flags]

View system audit logs.

Available Commands:
  list        List audit entries
  export      Export audit logs

Examples:
  # List recent audit entries
  vcdeploy audit list

  # Filter by user
  vcdeploy audit list --user admin

  # Export to CSV
  vcdeploy audit export --format csv --output audit.csv
```

---

### Utility Commands

#### completion

```
vcdeploy completion <shell> [flags]

Generate shell completion scripts.

Supported shells: bash, zsh, fish, powershell

Examples:
  # Bash (add to ~/.bashrc)
  source <(vcdeploy completion bash)

  # Zsh (add to ~/.zshrc)
  source <(vcdeploy completion zsh)

  # Fish
  vcdeploy completion fish | source

  # PowerShell
  vcdeploy completion powershell | Out-String | Invoke-Expression
```

#### version

```
vcdeploy version [flags]

Print version information.

Flags:
      --short   Print version number only

Examples:
  vcdeploy version
  vcdeploy version --short
```

#### admin

```
vcdeploy admin [flags]

Administrative lockout recovery.

Used to reset admin credentials when locked out of the web UI.
Requires direct server access.

Examples:
  # Reset admin password
  vcdeploy admin --reset-password
```

---

## Help Text Standards

### Cobra Field Usage

```go
var exampleCmd = &cobra.Command{
    Use:   "create <name> [flags]",           // Required args in <>, optional in []
    Short: "Create a new resource",            // One line, no period
    Long: `Create a new resource with the specified configuration.

Detailed explanation of what the command does, when to use it,
and any important caveats.

Related commands:
  vcdeploy resource show     Show resource details
  vcdeploy resource delete   Delete a resource`,
    Example: `  # Basic usage
  vcdeploy resource create myresource

  # With options
  vcdeploy resource create myresource --option value

  # From file
  vcdeploy resource create myresource -f config.yaml`,
    Aliases: []string{"add"},                  // Only if truly needed
    Args:    cobra.ExactArgs(1),
    RunE:    runCreate,
}
```

### Example Formatting

- Start each example with `#` comment explaining what it does
- Use 2-space indentation
- Show common use cases first, advanced later
- Include real-looking values, not `<placeholders>`

### Long Description Structure

1. First line: What the command does
2. Blank line
3. Detailed explanation
4. Blank line
5. Related commands (if helpful)
---

## Appendix: Removed Commands

These commands are **removed** in favor of the canonical patterns documented above.

| Removed Command | Replacement | Reason |
|-----------------|-------------|--------|
| `vcdeploy settings *` | `vcdeploy config *` | Standardized naming |
| `vcdeploy rollback *` | `vcdeploy project rollback *` | Noun-verb consistency |
| `vcdeploy deploy trigger` | `vcdeploy project deploy` | Centralize under resource |
| `vcdeploy totp *` | `vcdeploy user totp *` | User-scoped organization |
| `vcdeploy provision *` | `vcdeploy agent provision *` | Agent-scoped organization |
| `vcdeploy show <resource>` | `vcdeploy <resource> show` | Noun-verb consistency |

---

## Appendix: Verb Standardization

These verb changes align all commands to the standard CRUD pattern.

| Old Verb | New Verb | Affected Commands |
|----------|----------|-------------------|
| `add` | `create` | `blocked-ip`, `host-key`, `webhook` |
| `remove` | `delete` | `blocked-ip`, `host-key` |
| `status` | `show` | All resources |
| `get` | `show` | `recipe`, `secret`, `config` |
| `public` | `show` | `ssh-key public` → `ssh-key show` |
| `passwd` | `password` | `user passwd` → `user password` |

### Command Renames

| Old Command | New Command | Reason |
|-------------|-------------|--------|
| `certs` | `cert` | Singular nouns |
| `apikey` | `api-key` | Kebab-case consistency |

---

## Appendix: File Renames

Implementation detail — these are the source file renames in `cmd/vcdeploy/commands/`:

| Old File | New File |
|----------|----------|
| `admin_user.go` | `user.go` |
| `admin_agent.go` | `agent.go` |
| `admin_apikey.go` | `api_key.go` |
| `admin_deploy.go` | `deploy.go` |
| `admin_config.go` | `config.go` |
| `certificates.go` | `cert.go` |
| `ssh_keys.go` | `ssh_key.go` |
| `recipes.go` | `recipe.go` |

---

## Appendix: SSH Passphrase Support

Commands that use SSH keys support encrypted keys with passphrases:

### Interactive Prompt

When an SSH key requires a passphrase and a TTY is available:
```
Enter passphrase for SSH key: 
```

### File-Based Passphrase

For non-interactive/CI usage:
```bash
vcdeploy project deploy myapp --passphrase-file /path/to/passphrase
```

### Environment Variable

SSH_ASKPASS is respected:
```bash
export SSH_ASKPASS=/usr/bin/ksshaskpass
vcdeploy project deploy myapp
```

### Error Handling

When passphrase required but unavailable:
```
Error: SSH key requires passphrase but no TTY available; use --passphrase-file
```