# Project Configuration

Projects define what applications get deployed, where, and how.

## Overview

vcdeploy uses YAML files to configure projects. Each project defines:

- **Repository**: Source code location
- **Targets**: Where to deploy (agent-managed servers or master-local)
- **Deployment**: Strategy and behavior
- **Hooks**: Commands to run during deployment
- **Health checks**: Verification after deployment
- **Notifications**: Alerts on success/failure

## Project File Location

Project files are stored in the `projects/` directory under vcdeploy's data path:

```
/var/lib/vcdeploy/projects/
├── myapp.yaml
├── api-service.yaml
└── frontend.yaml
```

## Creating Projects

### Via CLI

```bash
# Create a basic project
vcdeploy project create myapp \
  --repo https://github.com/org/myapp \
  --branch main

# List projects
vcdeploy project list

# Show project details
vcdeploy project show myapp
```

### Via API

```bash
curl -X POST http://localhost:9000/api/v1/projects \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "myapp",
    "repository": "https://github.com/org/myapp",
    "branch": "main",
    "deploy_path": "/var/www/myapp",
    "type": "nodejs-app"
  }'
```

### Via Web UI

1. Navigate to **Projects** → **New Project**
2. Enter project name and repository URL
3. Configure targets and deployment settings
4. Save

## Full Configuration Reference

```yaml
# Project identification
name: "myapp"
type: "nodejs-app"              # Optional: Reference a project type template
repository: "https://github.com/org/myapp"
archived: false                  # Set true to disable deployments

# Branch watching
watch:
  branches:
    - "main"
    - "develop"
  actions:
    - "push"
    - "tag"
  guards:
    reject_force_push: true      # Default: true - Block force pushes
    require_ci_pass: false       # Require CI to pass before deploy

# Deployment behavior
deployment:
  on_busy: "cancel"              # cancel | queue | skip
  strategy: "symlink"            # symlink | inplace
  keep_releases: 5               # Number of releases to retain

# Environment file handling
env:
  template: ".env.template"      # Template file in repo
  placeholder_pattern: "${SECRET_NAME}"  # Pattern for secret substitution
  required_keys:
    - "DATABASE_URL"
    - "API_KEY"

# Deployment targets (named)
targets:
  production:
    agent: "prod-agent-01"       # Agent name (omit for master-local)
    path: "/var/www/myapp"
  
  staging:
    agent: "staging-01"
    path: "/var/www/myapp-staging"
  
  local:
    path: "/opt/deploy/myapp"    # No agent = master-local deployment

# Deployment orchestration (for multi-target deployments)
orchestration:
  mode: "parallel"               # parallel | rolling
  continue_on_error: false       # Continue to remaining targets on failure

# Deployment hooks
hooks:
  pre_deploy:
    - "php artisan down"
    - "./scripts/pre-deploy.sh"
  post_deploy:
    - "php artisan migrate --force"
    - "php artisan up"
  reload:
    - service: "nginx"
      action: "reload"
    - service: "php-fpm"
      action: "restart"
  rollback:
    - "./scripts/rollback-notify.sh"

# Health checks
health:
  url: "http://localhost:8080/health"    # Your application's port, not vcdeploy's
  timeout: "30s"
  retries: 3
  rollback_on_fail: true         # Auto-rollback if health check fails

# Notifications
notifications:
  on_success:
    - slack: "#deployments"
    - email: "team@example.com"
  on_failure:
    - slack: "#alerts"
    - email: "oncall@example.com"
    - webhook: "https://pagerduty.com/hook/xxx"
```

## Targets Configuration

Targets define where your application gets deployed. Each target is a named environment.

### Agent-Based Targets

Deploy through vcdeploy agents installed on target servers:

```yaml
targets:
  # Single agent target
  production:
    agent: "prod-server-01"
    path: "/var/www/myapp"

  # Multiple targets with different agents
  staging:
    agent: "staging-01"
    path: "/var/www/myapp-staging"

  canary:
    agent: "canary-01"
    path: "/var/www/myapp"
```

### Master-Local Targets

Deploy directly on the master server without an agent:

```yaml
targets:
  local:
    path: "/opt/deploy/myapp"     # No agent = master-local deployment
```

> **Note:** Targets defined in YAML config are synced to the database on project load. They can also be managed via CLI (`vcdeploy target create`) or API.

## Deployment Strategies

### Symlink Strategy (Default)

Creates versioned release directories with atomic symlink switching:

```
/var/www/myapp/
├── current -> releases/20240115120000  # Symlink to active release
├── releases/
│   ├── 20240115120000/                 # Current release
│   ├── 20240114100000/                 # Previous release
│   └── 20240113090000/                 # Older release
└── shared/                              # Shared files/directories
    ├── logs/
    └── .env
```

```yaml
deployment:
  strategy: "symlink"
  keep_releases: 5               # Keep last 5 releases for rollback
```

### Inplace Strategy

Deploys directly to the target path, overwriting existing files:

```yaml
deployment:
  strategy: "inplace"
```

Use inplace for:
- Simple static sites
- Applications without rollback requirements
- Development environments

## Deployment Behavior

### On Busy Handling

Controls what happens when a deployment is triggered while another is running:

```yaml
deployment:
  on_busy: "cancel"    # Cancel the new deployment (default)
  on_busy: "queue"     # Queue the new deployment to run after current
  on_busy: "skip"      # Skip silently, no notification
```

## Hooks

Hooks run commands at specific points in the deployment lifecycle:

```yaml
hooks:
  # Before deployment starts
  pre_deploy:
    - "php artisan down --render='errors::503'"
    - "npm run pre-deploy"

  # After files are deployed
  post_deploy:
    - "php artisan migrate --force"
    - "php artisan cache:clear"
    - "php artisan up"

  # Service management
  reload:
    - service: "nginx"
      action: "reload"           # reload | restart | start | stop
    - service: "php8.2-fpm"
      action: "restart"
    - service: "myapp"
      action: "restart"

  # On rollback (manual or automatic)
  rollback:
    - "./scripts/rollback-notification.sh"
    - "php artisan migrate:rollback"
```

### Hook Execution Order

1. `pre_deploy` - Before any files are transferred
2. Files transferred and prepared
3. `post_deploy` - After files deployed, before symlink switch (symlink strategy)
4. Symlink switched (atomic cutover)
5. `reload` - Service restarts/reloads
6. Health checks run
7. `rollback` - Only if rollback triggered

## Environment Variables

### Template Files

Use a template file to manage environment-specific configuration:

```yaml
env:
  template: ".env.template"
  placeholder_pattern: "${SECRET_NAME}"
  required_keys:
    - "DATABASE_URL"
    - "REDIS_URL"
    - "API_KEY"
```

Template file (`.env.template`):
```bash
APP_NAME=MyApp
APP_ENV=production

DATABASE_URL=${DATABASE_URL}
REDIS_URL=${REDIS_URL}
API_KEY=${API_KEY}
```

Secrets are substituted from vcdeploy's secret storage at deploy time.

## Health Checks

Post-deployment health verification:

```yaml
health:
  url: "http://localhost:8080/health"    # Your application's port, not vcdeploy's
  timeout: "30s"                 # Timeout per check attempt
  retries: 3                     # Number of retry attempts
  rollback_on_fail: true         # Auto-rollback on health check failure
```

Health check endpoints should return:
- **2xx status**: Healthy
- **Any other status**: Unhealthy

## Notifications

Configure alerts for deployment events:

```yaml
notifications:
  on_success:
    - slack: "#deployments"
    - email: "team@example.com"

  on_failure:
    - slack: "#alerts"
    - email: "oncall@example.com"
    - webhook: "https://hooks.example.com/deploy"
```

### Notification Channels

| Channel | Configuration | Notes |
|---------|--------------|-------|
| Slack | `slack: "#channel"` | Uses webhook configured in master |
| Email | `email: "addr@example.com"` | Uses SMTP from master config |
| Webhook | `webhook: "https://..."` | POST with deployment payload |

## Project Types

Project types provide reusable templates for common application patterns:

```yaml
# Use a project type
name: "myapp"
type: "nodejs-app"

# The project inherits settings from nodejs-app type
# You can override specific settings:
hooks:
  post_deploy:
    - "npm run custom-script"
```

### Managing Project Types

```bash
# List available types
vcdeploy project-type list

# Show type details
vcdeploy project-type show nodejs-app
```

## Branch Watch Guards

Protect deployments with guards:

```yaml
watch:
  branches:
    - "main"
  guards:
    reject_force_push: true      # Block force-pushed commits
    require_ci_pass: false       # Require CI status checks
```

## Archiving Projects

Disable deployments without deleting configuration:

```yaml
archived: true
```

Or via CLI:
```bash
vcdeploy project archive myapp
vcdeploy project unarchive myapp
```

## Triggering Deployments

### Via CLI

```bash
# Deploy specific target
vcdeploy deploy create --project myapp --target production --branch main

# Deploy with specific commit
vcdeploy deploy create --project myapp --target production --commit abc123
```

### Via Webhook

Configure webhooks from Git providers (GitHub, GitLab, Bitbucket) to automatically trigger deployments on push.

### Via API

```bash
# Create deployment via API
curl -X POST http://localhost:9000/api/v1/deployments \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"project": "myapp", "target": "production", "branch": "main"}'
```

## Example Configurations

### Node.js Application

```yaml
name: "api-service"
repository: "https://github.com/org/api-service"

watch:
  branches: ["main"]
  guards:
    reject_force_push: true

deployment:
  strategy: "symlink"
  keep_releases: 5
  on_busy: "queue"

targets:
  production:
    agent: "api-prod-01"
    path: "/var/www/api"
  production-2:
    agent: "api-prod-02"
    path: "/var/www/api"

orchestration:
  mode: "rolling"

hooks:
  pre_deploy:
    - "npm ci --production"
    - "npm run build"
  post_deploy:
    - "npm run db:migrate"
  reload:
    - service: "api-service"
      action: "restart"

health:
  url: "http://localhost:3000/health"
  timeout: "30s"
  retries: 3
  rollback_on_fail: true

notifications:
  on_success:
    - slack: "#deployments"
  on_failure:
    - slack: "#oncall"
```

### PHP/Laravel Application

```yaml
name: "laravel-app"
repository: "git@github.com:org/laravel-app.git"

deployment:
  strategy: "symlink"
  keep_releases: 10

targets:
  production:
    agent: "web-prod-01"
    path: "/var/www/laravel"

env:
  template: ".env.production"
  required_keys:
    - "APP_KEY"
    - "DB_PASSWORD"

hooks:
  pre_deploy:
    - "composer install --no-dev --optimize-autoloader"
    - "php artisan down"
  post_deploy:
    - "php artisan migrate --force"
    - "php artisan config:cache"
    - "php artisan route:cache"
    - "php artisan view:cache"
    - "php artisan up"
  reload:
    - service: "php8.2-fpm"
      action: "reload"

health:
  url: "http://localhost/up"
  rollback_on_fail: true
```

### Static Site

```yaml
name: "docs-site"
repository: "https://github.com/org/docs"

deployment:
  strategy: "inplace"

targets:
  production:
    agent: "docs-server"
    path: "/var/www/docs"

hooks:
  pre_deploy:
    - "npm ci"
    - "npm run build"
```

## Database Integration

Projects are stored in vcdeploy's database with the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `id` | int64 | Unique identifier |
| `name` | string | Project name (unique) |
| `repository` | string | Git repository URL |
| `branch` | string | Default branch |
| `deploy_path` | string | Default deployment path |
| `type` | string | Project type reference |
| `auto_rollback_enabled` | bool | Enable automatic rollback |
| `rollback_on_health_fail` | bool | Rollback on health check failure |
| `last_deploy_at` | timestamp | Last deployment time |
| `last_deploy_status` | string | Status of last deployment |

## Related Documentation

- [Master Configuration](config/master.md) - Server and global settings
- [Agent Configuration](config/agent.md) - Agent setup
- [Secrets Management](config/secrets.md) - Managing sensitive data
- [Webhooks](api/webhooks.md) - Git provider integration
