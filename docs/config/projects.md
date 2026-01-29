# Project Configuration

Projects define what gets deployed and how.

## Creating a Project

### Via CLI

```bash
vcdeploy project create myapp \
  --repo https://github.com/org/myapp \
  --branch main \
  --path /var/www/myapp
```

### Via Web UI

1. Navigate to **Projects** → **New Project**
2. Fill in project details
3. Configure deployment settings
4. Save and activate

## Project Settings

```yaml
# Project configuration
name: "myapp"
description: "My Application"

# Repository settings
repository:
  url: "https://github.com/org/myapp"
  branch: "main"
  webhook_secret: "your-webhook-secret"

# Deployment target
deploy:
  path: "/var/www/myapp"
  keep_releases: 5
  
  # Target agents by label
  targets:
    - agent_labels:
        environment: "production"

# Build commands
build:
  commands:
    - "npm install"
    - "npm run build"

# Deployment hooks
hooks:
  before_deploy:
    - "php artisan down"
  after_deploy:
    - "php artisan migrate --force"
    - "php artisan cache:clear"
    - "php artisan up"

# Shared files/directories
shared:
  dirs:
    - "logs"
    - "storage"
  files:
    - ".env"

# Environment variables
environment:
  NODE_ENV: "production"
  APP_DEBUG: "false"
```

## Repository Types

### Public Repositories

```yaml
repository:
  url: "https://github.com/org/myapp"
```

### Private Repositories (SSH)

```yaml
repository:
  url: "git@github.com:org/myapp.git"
  ssh_key: "deploy-key-name"  # Reference to stored SSH key
```

### Private Repositories (Token)

```yaml
repository:
  url: "https://github.com/org/myapp"
  credentials: "github-token"  # Reference to stored credential
```

## Deployment Targets

### By Agent ID

```yaml
deploy:
  targets:
    - agent_id: "agent-001"
    - agent_id: "agent-002"
```

### By Labels

```yaml
deploy:
  targets:
    - agent_labels:
        environment: "production"
        tier: "web"
```

### SSH Targets (No Agent)

```yaml
deploy:
  targets:
    - ssh:
        host: "server.example.com"
        port: 22
        user: "deploy"
        key: "server-ssh-key"
```

## Build Commands

```yaml
build:
  commands:
    # Node.js
    - "npm ci"
    - "npm run build"
    
    # PHP/Composer
    - "composer install --no-dev --optimize-autoloader"
    
    # Python
    - "pip install -r requirements.txt"
    
    # Custom
    - "./scripts/build.sh"
```

## Deployment Hooks

```yaml
hooks:
  # Before deployment starts
  before_deploy:
    - "php artisan down"
    - "./scripts/pre-deploy.sh"
    
  # After files deployed, before symlink
  before_symlink:
    - "./scripts/verify.sh"
    
  # After symlink updated
  after_deploy:
    - "php artisan migrate --force"
    - "php artisan up"
    - "sudo systemctl reload nginx"
    
  # On deployment failure
  on_failure:
    - "./scripts/notify-failure.sh"
```

## Environment Variables

### Inline

```yaml
environment:
  NODE_ENV: "production"
  DATABASE_URL: "postgres://..."
```

### From Secrets

```yaml
environment:
  DATABASE_URL: "${secret:database_url}"
  API_KEY: "${secret:api_key}"
```

## Notifications

```yaml
notifications:
  slack:
    channel: "#deployments"
    on_start: true
    on_success: true
    on_failure: true
  email:
    recipients:
      - "team@example.com"
    on_failure: true
```

## Webhook Configuration

### GitHub

```yaml
webhooks:
  github:
    secret: "your-webhook-secret"
    events:
      - "push"
      - "pull_request"
    branches:
      - "main"
      - "develop"
```

### GitLab

```yaml
webhooks:
  gitlab:
    token: "your-webhook-token"
    events:
      - "push"
      - "merge_request"
```

## Project Types

Use project types for common configurations:

```yaml
# Reference a project type
type: "nodejs-app"

# Override specific settings
build:
  commands:
    - "npm ci"
    - "npm run build:prod"  # Custom build
```
