#!/usr/bin/env python3
"""Add uid to memory store write operations and Create methods."""

STORAGE = "/opt/code/vcdeploy/internal/storage"

def process_file(filepath, replacements):
    with open(filepath, 'r') as f:
        content = f.read()
    original = content
    for desc, old, new in replacements:
        if old not in content:
            print(f"  WARNING not found: {desc}")
            continue
        count = content.count(old)
        content = content.replace(old, new)
        print(f"  OK ({count}x): {desc}")
    if content != original:
        with open(filepath, 'w') as f:
            f.write(content)
        print(f"  >> WRITTEN: {filepath}")
    else:
        print(f"  >> NO CHANGES: {filepath}")

# ===================================================================
# memory_writeops.go - Add uid to INSERT SQL + args
# ===================================================================
print("\n=== memory_writeops.go ===")
process_file(f"{STORAGE}/memory_writeops.go", [
    # Users INSERT
    ("users INSERT cols",
     'INSERT INTO users (id, username, password_hash, email, role, must_change_password, totp_secret, totp_enabled, created_at, updated_at)',
     'INSERT INTO users (id, uid, username, password_hash, email, role, must_change_password, totp_secret, totp_enabled, created_at, updated_at)'),
    ("users VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(id)',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(id)'),
    ("users args",
     '`, user.ID, user.Username, user.PasswordHash, user.Email, user.Role,',
     '`, user.ID, user.UID, user.Username, user.PasswordHash, user.Email, user.Role,'),
    # Users ON CONFLICT - add uid to UPDATE SET
    ("users ON CONFLICT uid",
     'ON CONFLICT(id) DO UPDATE SET\n\t\t\t\tusername = excluded.username,',
     'ON CONFLICT(id) DO UPDATE SET\n\t\t\t\tuid = excluded.uid,\n\t\t\t\tusername = excluded.username,'),

    # API Keys INSERT
    ("apikeys INSERT cols",
     'INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at)',
     'INSERT INTO api_keys (id, uid, user_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at)'),
    ("apikeys VALUES",
     "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, key.ID, key.UserID,",
     "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, key.ID, key.UID, key.UserID,"),

    # Settings INSERT
    ("settings INSERT cols",
     'INSERT INTO settings (id, category, key, value, value_type, encrypted, description, created_at, updated_at)',
     'INSERT INTO settings (id, uid, category, key, value, value_type, encrypted, description, created_at, updated_at)'),
    ("settings VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(category, key)',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(category, key)'),
    ("settings args",
     '`, setting.ID, setting.Category, setting.Key, setting.Value, setting.ValueType, setting.Encrypted, setting.Description, setting.CreatedAt, setting.UpdatedAt)',
     '`, setting.ID, setting.UID, setting.Category, setting.Key, setting.Value, setting.ValueType, setting.Encrypted, setting.Description, setting.CreatedAt, setting.UpdatedAt)'),

    # Projects INSERT
    ("projects INSERT cols",
     'INSERT INTO projects (id, name, repository, branch, deploy_path, type, created_at, updated_at,',
     'INSERT INTO projects (id, uid, name, repository, branch, deploy_path, type, created_at, updated_at,'),
    ("projects VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, p.ID, p.Name,',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, p.ID, p.UID, p.Name,'),

    # Project Types INSERT
    ("project_types INSERT cols",
     'INSERT INTO project_types (id, name, description, build_cmd, project_count, created_at)',
     'INSERT INTO project_types (id, uid, name, description, build_cmd, project_count, created_at)'),
    ("project_types VALUES",
     "VALUES (?, ?, ?, ?, ?, ?)\n\t\t`, pt.ID, pt.Name,",
     "VALUES (?, ?, ?, ?, ?, ?, ?)\n\t\t`, pt.ID, pt.UID, pt.Name,"),

    # Webhooks INSERT
    ("webhooks INSERT cols",
     'INSERT INTO project_webhooks (id, project_id, provider, secret_encrypted, enabled, require_secret, created_at, updated_at)',
     'INSERT INTO project_webhooks (id, uid, project_id, provider, secret_encrypted, enabled, require_secret, created_at, updated_at)'),
    ("webhooks VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(project_id, provider)',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(project_id, provider)'),
    ("webhooks args",
     '`, wh.ID, wh.ProjectID, wh.Provider, wh.SecretEncrypted, wh.Enabled, wh.RequireSecret, wh.CreatedAt, wh.UpdatedAt)',
     '`, wh.ID, wh.UID, wh.ProjectID, wh.Provider, wh.SecretEncrypted, wh.Enabled, wh.RequireSecret, wh.CreatedAt, wh.UpdatedAt)'),

    # Secrets INSERT
    ("secrets INSERT cols",
     'INSERT INTO secrets (id, project, project_id, scope, key, value_encrypted, created_at, updated_at)',
     'INSERT INTO secrets (id, uid, project, project_id, scope, key, value_encrypted, created_at, updated_at)'),
    ("secrets VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(project, scope, key)',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(project, scope, key)'),
    ("secrets args",
     '`, secret.ID, secret.Project, secret.ProjectID, secret.Scope, secret.Key, secret.ValueEncrypted, secret.CreatedAt, secret.UpdatedAt)',
     '`, secret.ID, secret.UID, secret.Project, secret.ProjectID, secret.Scope, secret.Key, secret.ValueEncrypted, secret.CreatedAt, secret.UpdatedAt)'),

    # SSH Host Keys INSERT
    ("ssh_host_keys INSERT cols",
     'INSERT INTO ssh_host_keys (id, hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at, created_at, updated_at)',
     'INSERT INTO ssh_host_keys (id, uid, hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at, created_at, updated_at)'),
    ("ssh_host_keys VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, k.ID, k.Hostname,',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, k.ID, k.UID, k.Hostname,'),

    # Jump Servers INSERT
    ("jump_servers INSERT cols",
     'INSERT INTO ssh_jump_servers (id, name, host, port, username, ssh_key_id, created_at)',
     'INSERT INTO ssh_jump_servers (id, uid, name, host, port, username, ssh_key_id, created_at)'),
    ("jump_servers VALUES",
     "VALUES (?, ?, ?, ?, ?, ?, ?)\n\t\t`, j.ID, j.Name,",
     "VALUES (?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, j.ID, j.UID, j.Name,"),

    # Health Check Configs INSERT
    ("health_check INSERT cols",
     'INSERT INTO health_check_configs (id, project_id, name, url, method, expected_status, timeout_seconds,',
     'INSERT INTO health_check_configs (id, uid, project_id, name, url, method, expected_status, timeout_seconds,'),
    ("health_check VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, c.ID, c.ProjectID,',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, c.ID, c.UID, c.ProjectID,'),

    # ACME Certificates INSERT
    ("acme_certs INSERT cols",
     'INSERT INTO acme_certificates (id, domain, certificate_pem, private_key_encrypted, issuer,',
     'INSERT INTO acme_certificates (id, uid, domain, certificate_pem, private_key_encrypted, issuer,'),
    ("acme_certs VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(domain)',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t\tON CONFLICT(domain)'),
    ("acme_certs args",
     '`, c.ID, c.Domain, c.CertificatePEM, c.PrivateKeyEncrypted, c.Issuer,',
     '`, c.ID, c.UID, c.Domain, c.CertificatePEM, c.PrivateKeyEncrypted, c.Issuer,'),

    # ACME Accounts INSERT
    ("acme_accounts INSERT cols",
     'INSERT INTO acme_accounts (id, email, account_url, private_key_encrypted, directory_url, created_at)',
     'INSERT INTO acme_accounts (id, uid, email, account_url, private_key_encrypted, directory_url, created_at)'),
    ("acme_accounts VALUES",
     "VALUES (?, ?, ?, ?, ?, ?)\n\t\t`, a.ID, a.Email,",
     "VALUES (?, ?, ?, ?, ?, ?, ?)\n\t\t`, a.ID, a.UID, a.Email,"),

    # Recipe Components INSERT
    ("recipe_components INSERT cols",
     'INSERT INTO recipe_components (id, namespace, slug, version, name, description,',
     'INSERT INTO recipe_components (id, uid, namespace, slug, version, name, description,'),
    ("recipe_components VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, c.ID, c.Namespace,',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, c.ID, c.UID, c.Namespace,'),

    # Playbooks INSERT
    ("playbooks INSERT cols",
     'INSERT INTO playbooks (id, namespace, slug, version, name, description, framework_type,',
     'INSERT INTO playbooks (id, uid, namespace, slug, version, name, description, framework_type,'),
    ("playbooks VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, p.ID, p.Namespace,',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, p.ID, p.UID, p.Namespace,'),

    # Playbook Activations INSERT
    ("activations INSERT cols",
     'INSERT INTO playbook_activations (id, project_id, playbook_id, activated_at, activated_by)',
     'INSERT INTO playbook_activations (id, uid, project_id, playbook_id, activated_at, activated_by)'),
    ("activations VALUES",
     "VALUES (?, ?, ?, ?, ?)\n\t\t`, a.ID, a.ProjectID,",
     "VALUES (?, ?, ?, ?, ?, ?)\n\t\t`, a.ID, a.UID, a.ProjectID,"),

    # Variable Bindings INSERT
    ("bindings INSERT cols",
     'INSERT INTO playbook_variable_bindings (id, activation_id, variable_name, source_type, source_ref, literal_value)',
     'INSERT INTO playbook_variable_bindings (id, uid, activation_id, variable_name, source_type, source_ref, literal_value)'),
    ("bindings VALUES",
     "VALUES (?, ?, ?, ?, ?, ?)\n\t\t`, b.ID, b.ActivationID,",
     "VALUES (?, ?, ?, ?, ?, ?, ?)\n\t\t`, b.ID, b.UID, b.ActivationID,"),

    # Raw Approvals INSERT
    ("approvals INSERT cols",
     'INSERT INTO raw_command_approvals (id, component_id, approved_by, approved_at, approval_note)',
     'INSERT INTO raw_command_approvals (id, uid, component_id, approved_by, approved_at, approval_note)'),
    ("approvals VALUES",
     "VALUES (?, ?, ?, ?, ?)\n\t\t`, a.ID, a.ComponentID,",
     "VALUES (?, ?, ?, ?, ?, ?)\n\t\t`, a.ID, a.UID, a.ComponentID,"),
])

# ===================================================================
# memory_users.go - Add uid auto-gen to CreateUser and CreateAPIKey
# ===================================================================
print("\n=== memory_users.go ===")
process_file(f"{STORAGE}/memory_users.go", [
    # Import
    ("xid import",
     '"context"\n\t"time"\n)',
     '"context"\n\t"time"\n\n\t"github.com/rs/xid"\n)'),

    # CreateUser uid auto-gen (before ID assignment)
    ("CreateUser uid",
     'user.ID = nextID(&s.nextUserID)\n\tnow := time.Now()',
     'if user.UID == "" {\n\t\tuser.UID = xid.New().String()\n\t}\n\tuser.ID = nextID(&s.nextUserID)\n\tnow := time.Now()'),

    # CreateAPIKey uid auto-gen
    ("CreateAPIKey uid",
     'key.ID = nextID(&s.nextAPIKeyID)\n\tkey.CreatedAt = time.Now()',
     'if key.UID == "" {\n\t\tkey.UID = xid.New().String()\n\t}\n\tkey.ID = nextID(&s.nextAPIKeyID)\n\tkey.CreatedAt = time.Now()'),
])

# ===================================================================
# memory_projects.go - CreateProject, CreateProjectType, SetProjectWebhook, SetSecretEncrypted
# ===================================================================
print("\n=== memory_projects.go ===")
process_file(f"{STORAGE}/memory_projects.go", [
    # Import
    ("xid import",
     '"context"\n\t"time"\n)',
     '"context"\n\t"time"\n\n\t"github.com/rs/xid"\n)'),

    # CreateProject uid auto-gen
    ("CreateProject uid",
     'project.ID = nextID(&s.nextProjectID)\n\tnow := time.Now()',
     'if project.UID == "" {\n\t\tproject.UID = xid.New().String()\n\t}\n\tproject.ID = nextID(&s.nextProjectID)\n\tnow := time.Now()'),

    # CreateProjectType uid auto-gen
    ("CreateProjectType uid",
     'pt.ID = nextID(&s.nextProjectTypeID)\n\tpt.CreatedAt = time.Now()',
     'if pt.UID == "" {\n\t\tpt.UID = xid.New().String()\n\t}\n\tpt.ID = nextID(&s.nextProjectTypeID)\n\tpt.CreatedAt = time.Now()'),

    # SetProjectWebhook - add UID to new webhook struct literal
    ("SetProjectWebhook uid",
     'webhook := &ProjectWebhook{\n\t\tID:              nextID(&s.nextWebhookID),',
     'webhook := &ProjectWebhook{\n\t\tUID:             xid.New().String(),\n\t\tID:              nextID(&s.nextWebhookID),'),

    # SetSecretEncrypted - add UID to new secret struct literal
    ("SetSecretEncrypted uid",
     'secret := &Secret{\n\t\tID:             nextID(&s.nextSecretID),',
     'secret := &Secret{\n\t\tUID:            xid.New().String(),\n\t\tID:             nextID(&s.nextSecretID),'),
])

# ===================================================================
# memory_misc.go - CreateSSHHostKey, CreateJumpServer, CreateHealthCheckConfig
# ===================================================================
print("\n=== memory_misc.go ===")
process_file(f"{STORAGE}/memory_misc.go", [
    # Import
    ("xid import",
     '"context"\n\t"time"\n)',
     '"context"\n\t"time"\n\n\t"github.com/rs/xid"\n)'),

    # CreateSSHHostKey uid auto-gen
    ("CreateSSHHostKey uid",
     'key.ID = nextID(&s.nextSSHHostKeyID)\n\tnow := time.Now()',
     'if key.UID == "" {\n\t\tkey.UID = xid.New().String()\n\t}\n\tkey.ID = nextID(&s.nextSSHHostKeyID)\n\tnow := time.Now()'),

    # CreateJumpServer uid auto-gen
    ("CreateJumpServer uid",
     'js.ID = nextID(&s.nextJumpServerID)\n\tjs.CreatedAt = time.Now()',
     'if js.UID == "" {\n\t\tjs.UID = xid.New().String()\n\t}\n\tjs.ID = nextID(&s.nextJumpServerID)\n\tjs.CreatedAt = time.Now()'),

    # CreateHealthCheckConfig uid auto-gen
    ("CreateHealthCheckConfig uid",
     'config.ID = nextID(&s.nextHealthCheckID)\n\tnow := time.Now()',
     'if config.UID == "" {\n\t\tconfig.UID = xid.New().String()\n\t}\n\tconfig.ID = nextID(&s.nextHealthCheckID)\n\tnow := time.Now()'),
])

# ===================================================================
# memory_audit.go - SetSetting
# ===================================================================
print("\n=== memory_audit.go ===")
process_file(f"{STORAGE}/memory_audit.go", [
    # Check if xid import already exists, if not add it
    # The file probably imports "context", "time" or similar
    # Let's try the common import pattern
    ("xid import try1",
     '"context"\n\t"time"\n)',
     '"context"\n\t"time"\n\n\t"github.com/rs/xid"\n)'),

    # SetSetting - add UID to new setting struct literal
    ("SetSetting uid",
     'setting := &Setting{\n\t\tID:        nextID(&s.nextSettingID),',
     'setting := &Setting{\n\t\tUID:       xid.New().String(),\n\t\tID:        nextID(&s.nextSettingID),'),
])

# ===================================================================
# memory_security.go (MemoryStore Save methods at top of file)
# xid import was already added by the first script
# ===================================================================
print("\n=== memory_security.go (MemoryStore methods) ===")
process_file(f"{STORAGE}/memory_security.go", [
    # SaveCA - MemoryStore version (line ~59)
    # Need to find the MemoryStore SaveCA and add uid auto-gen
    # The MemoryStore SaveCA stores a copy, let me find its pattern
    # Looking at memory_security.go MemoryStore methods around line 59
    # I need to search for the pattern specific to MemoryStore SaveCA
    # These are probably simple store-and-queue operations

    # For MemoryStore methods, the uid should be auto-generated when creating new records.
    # The MemoryStore Save methods typically check if the record exists and create/update.
    # Since these methods receive the struct from the caller, and the DB methods already
    # generate UIDs, we mainly need to handle cases where MemoryStore methods create new UIDs.
    # But actually, looking at the code, SaveCA on MemoryStore stores the CA as-is.
    # The DB SaveCA (already updated) generates UIDs. The MemoryStore SaveCA just stores in memory.
    # So we mainly need the MemoryStore Create methods that CREATE new records.
    
    # For MemoryStore, the Save methods just store whatever struct is passed.
    # The uid should already be set by the caller or by the DB Create methods.
    # However, for defensive programming, let's add uid generation to MemoryStore Save methods
    # that create new records (when ID == 0 or similar).

    # Actually, looking at the MemoryStore SaveCA code pattern, it likely just stores the CA.
    # The uid is set by the DB SaveCA method. Since both MemoryStore and DB implement the same
    # interface, and the uid generation is in the DB methods, the MemoryStore methods need it too.
    
    # But I don't have the exact code for MemoryStore SaveCA, SaveAgentCert, etc.
    # Let me skip these for now and handle them after the build test.
])

print("\n=== DONE ===")
print("Run 'go build ./...' to verify.")
