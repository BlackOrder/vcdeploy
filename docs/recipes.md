# Recipe System

The Recipe System provides a database-backed, version-controlled approach to deployment composition. Instead of editing YAML files directly, you can compose deployments using reusable components, organized into playbooks that can be activated per-project.

## Overview

### Key Concepts

- **Components**: Reusable deployment actions (e.g., "composer install", "npm build", "service reload")
- **Playbooks**: Ordered collections of components defining a complete deployment workflow
- **Activations**: Links between projects and their configured playbooks
- **Variable Bindings**: Runtime values passed to components during deployment

### Namespaces

Components and playbooks exist in two namespaces:

| Namespace | Description | Editable |
|-----------|-------------|----------|
| `seed` | Built-in, versioned components | Read-only (copy to customize) |
| `user` | Custom components | Full edit access |

## Components

Components are the building blocks of deployments. Each component performs a specific action during deployment.

### Component Types

| Type | Description | Security |
|------|-------------|----------|
| `shell` | Execute shell command | Validated |
| `reload` | Reload a service | Validated |
| `file_operation` | File system operations | Validated |
| `raw` | Arbitrary shell command | Requires Admin approval |

### Creating Components

**Via Web UI:**
1. Navigate to Recipes → Components
2. Click "New Component"
3. Fill in name, description, type
4. Define the command or action
5. Add variables for runtime configuration

**Via API:**
```bash
curl -X POST /api/v1/recipes/components \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "PHP-FPM Reload",
    "slug": "php-fpm-reload",
    "type": "reload",
    "content": {
      "service_name": "php-fpm",
      "signal": "reload"
    }
  }'
```

### Variables

Components can define variables that are filled in at runtime:

```json
{
  "variables": [
    {
      "name": "php_version",
      "type": "string",
      "default": "8.2",
      "description": "PHP version to use"
    }
  ]
}
```

Variable sources:
- `literal`: Static value
- `env`: Environment variable
- `secret`: Stored secret reference

## Playbooks

Playbooks combine components into deployment workflows with ordered steps across phases.

### Deployment Phases

| Phase | Description | Timing |
|-------|-------------|--------|
| `pre_deploy` | Before code changes | Before git pull |
| `deploy` | Main deployment | Symlink switch |
| `post_deploy` | After deployment | After symlink |
| `rollback` | Recovery actions | On failure |

### Creating Playbooks

**Via Web UI:**
1. Navigate to Recipes → Playbooks
2. Click "New Playbook"
3. Drag components into phases
4. Configure step order and conditions
5. Set variable bindings

**Via Playbook Composer:**
1. Navigate to project detail page
2. Click "Edit Steps" on active playbook
3. Use drag-and-drop composer
4. Save changes

### Step Configuration

Each step in a playbook can have:
- **Component Reference**: `namespace:slug:version`
- **Variable Bindings**: Map playbook variables to component variables
- **Conditions**: Skip step based on expression
- **Order**: Execution sequence within phase

## Activations

Activating a playbook connects it to a project, configuring how deployments run.

### Activating via Web UI

1. Go to Projects → [Project Name]
2. In the Playbook Configuration section, click "Activate Playbook"
3. Select a playbook from available options
4. Configure variable bindings (map to secrets, env vars, or literals)
5. Review dependency warnings
6. Click "Activate"

### Deactivating

To revert to YAML-based deployment:
1. Go to Projects → [Project Name]
2. Click "Deactivate" in the Playbook Configuration section
3. Confirm the action

## RAW Commands

RAW components allow arbitrary shell commands but require Admin approval for security.

### Creating RAW Components

1. Create component with type `raw`
2. Component is created in "pending" state
3. Admin must approve before it can be used in deployments

### Admin Approval

Admins can approve RAW commands via:
- Web UI: Recipes → Components → [RAW Component] → Approve
- API: `POST /api/v1/recipes/raw-approvals`

Approvals are logged with:
- Admin user ID
- Approval timestamp
- Optional approval note

## Migration from YAML

### CLI Migration

Import existing YAML configuration:

```bash
# Preview migration
vcdeploy recipes import-yaml configs/myproject.yaml --preview

# Import and create playbook
vcdeploy recipes import-yaml configs/myproject.yaml \
  --project-id 123 \
  --name "My Project Playbook" \
  --version "v1.0.0" \
  --create-components \
  --activate
```

### Web UI Migration

1. Go to Projects → [Project Name]
2. Click "Migrate YAML to Playbook" in Quick Actions
3. Review the migration preview
4. Configure options (name, version, auto-activate)
5. Click "Start Migration"

## Export/Import

Backup and share recipe configurations:

### Export

```bash
# Export all recipes
curl /api/v1/recipes/export \
  -H "Authorization: Bearer $TOKEN" \
  > recipes-backup.json

# Export with version history
curl "/api/v1/recipes/export?include_versions=true" \
  -H "Authorization: Bearer $TOKEN" \
  > recipes-full.json
```

### Import

```bash
# Dry run
curl -X POST /api/v1/recipes/import \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@recipes-backup.json" \
  -F "dry_run=true"

# Import
curl -X POST /api/v1/recipes/import \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@recipes-backup.json"
```

## Best Practices

### Component Design

1. **Single Responsibility**: One component = one action
2. **Variable Abstraction**: Use variables for values that change between projects
3. **Clear Naming**: Use descriptive names that indicate the action
4. **Version Management**: Use semver for component versions

### Playbook Design

1. **Phase Separation**: Keep actions in appropriate phases
2. **Idempotency**: Steps should be safe to re-run
3. **Rollback Planning**: Always include rollback steps
4. **Variable Documentation**: Document all required variables

### Security

1. **Avoid RAW**: Prefer typed components over RAW commands
2. **Review Approvals**: Carefully audit RAW command approvals
3. **Secret References**: Use secret bindings, not literal values
4. **Dependency Tracking**: Review warnings before deleting secrets/env vars

## Troubleshooting

### "Component not found" errors

- Check the component reference format: `namespace:slug:version`
- Verify the component exists in the specified namespace
- Check version compatibility

### RAW component "pending approval"

- Contact an admin to approve the RAW command
- Check the approval status in the component detail view

### Variable binding errors

- Verify the secret/env key exists
- Check variable type compatibility
- Review the dependency warnings

### Migration failures

- Ensure the YAML file is valid
- Check for unsupported hook types
- Review the migration preview for warnings
