#!/usr/bin/env python3
"""Add uid column to all SQL operations in the storage layer."""

import sys, os

STORAGE = "/opt/code/vcdeploy/internal/storage"

def process_file(filepath, replacements):
    """Apply a list of (description, old, new) replacements to a file."""
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
# db_projects.go
# ===================================================================
print("\n=== db_projects.go ===")
process_file(f"{STORAGE}/db_projects.go", [
    # --- Import ---
    ("xid import",
     '\t"go.uber.org/zap"\n)',
     '\t"github.com/rs/xid"\n\t"go.uber.org/zap"\n)'),

    # --- CreateProject ---
    ("CreateProject uid auto-gen",
     'func (db *DB) CreateProject(ctx context.Context, project *Project) error {\n\tresult, err :=',
     'func (db *DB) CreateProject(ctx context.Context, project *Project) error {\n\tif project.UID == "" {\n\t\tproject.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreateProject INSERT cols",
     'INSERT INTO projects (name, repository, branch, deploy_path, type, created_at)',
     'INSERT INTO projects (uid, name, repository, branch, deploy_path, type, created_at)'),
    ("CreateProject VALUES",
     'VALUES (?, ?, ?, ?, ?, ?)\n\t`, project.Name,',
     'VALUES (?, ?, ?, ?, ?, ?, ?)\n\t`, project.UID, project.Name,'),

    # --- Project SELECTs (4x) ---
    ("Project SELECT cols",
     'SELECT id, name, repository, branch, deploy_path, type, created_at, last_deploy_at, last_deploy_status',
     'SELECT id, uid, name, repository, branch, deploy_path, type, created_at, last_deploy_at, last_deploy_status'),

    # --- Project Scans (4x) ---
    ("Project Scan",
     '&p.ID, &p.Name, &p.Repository, &p.Branch, &p.DeployPath, &p.Type, &p.CreatedAt, &lastDeploy, &lastDeployStatus',
     '&p.ID, &p.UID, &p.Name, &p.Repository, &p.Branch, &p.DeployPath, &p.Type, &p.CreatedAt, &lastDeploy, &lastDeployStatus'),

    # --- CreateProjectType ---
    ("CreateProjectType uid auto-gen",
     'func (db *DB) CreateProjectType(ctx context.Context, pt *ProjectType) error {\n\tresult, err :=',
     'func (db *DB) CreateProjectType(ctx context.Context, pt *ProjectType) error {\n\tif pt.UID == "" {\n\t\tpt.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreateProjectType INSERT cols",
     'INSERT INTO project_types (name, description, build_cmd, created_at)',
     'INSERT INTO project_types (uid, name, description, build_cmd, created_at)'),
    ("CreateProjectType VALUES",
     "VALUES (?, ?, ?, ?)\n\t`, pt.Name,",
     "VALUES (?, ?, ?, ?, ?)\n\t`, pt.UID, pt.Name,"),

    # --- ProjectType SELECTs (2x) ---
    ("ProjectType SELECT cols",
     'SELECT pt.id, pt.name, pt.description, pt.build_cmd,',
     'SELECT pt.id, pt.uid, pt.name, pt.description, pt.build_cmd,'),

    # --- ProjectType Scans (2x) ---
    ("ProjectType Scan",
     '&pt.ID, &pt.Name, &pt.Description, &pt.BuildCmd, &pt.ProjectCount, &pt.CreatedAt',
     '&pt.ID, &pt.UID, &pt.Name, &pt.Description, &pt.BuildCmd, &pt.ProjectCount, &pt.CreatedAt'),

    # --- SetProjectWebhook ---
    ("SetProjectWebhook uid gen",
     'func (db *DB) SetProjectWebhook(ctx context.Context, projectID int64, provider string, secretEncrypted []byte, enabled, requireSecret bool) error {\n\tenabledVal :=',
     'func (db *DB) SetProjectWebhook(ctx context.Context, projectID int64, provider string, secretEncrypted []byte, enabled, requireSecret bool) error {\n\tuid := xid.New().String()\n\tenabledVal :='),
    ("SetProjectWebhook INSERT cols",
     'INSERT INTO project_webhooks (project_id, provider, secret_encrypted, enabled, require_secret)',
     'INSERT INTO project_webhooks (uid, project_id, provider, secret_encrypted, enabled, require_secret)'),
    ("SetProjectWebhook VALUES",
     "VALUES (?, ?, ?, ?, ?)\n\t\tON CONFLICT",
     "VALUES (?, ?, ?, ?, ?, ?)\n\t\tON CONFLICT"),
    ("SetProjectWebhook args",
     '`, projectID, provider, secretEncrypted, enabledVal, requireSecretVal)',
     '`, uid, projectID, provider, secretEncrypted, enabledVal, requireSecretVal)'),

    # --- Webhook SELECTs (2x) ---
    ("Webhook SELECT cols",
     'SELECT id, project_id, provider, secret_encrypted, enabled, COALESCE(require_secret, 0), created_at, updated_at',
     'SELECT id, uid, project_id, provider, secret_encrypted, enabled, COALESCE(require_secret, 0), created_at, updated_at'),

    # --- Webhook Scans (2x) ---
    ("Webhook Scan",
     '&w.ID, &w.ProjectID, &w.Provider',
     '&w.ID, &w.UID, &w.ProjectID, &w.Provider'),
])

# ===================================================================
# db_secrets.go
# ===================================================================
print("\n=== db_secrets.go ===")
process_file(f"{STORAGE}/db_secrets.go", [
    # --- Import ---
    ("xid import",
     '"context"\n\t"database/sql"\n\t"fmt"\n)',
     '"context"\n\t"database/sql"\n\t"fmt"\n\n\t"github.com/rs/xid"\n)'),

    # --- SetSecretEncrypted ---
    ("SetSecretEncrypted uid gen",
     'func (db *DB) SetSecretEncrypted(ctx context.Context, project, scope, key string, valueEncrypted []byte) error {\n\t_, err :=',
     'func (db *DB) SetSecretEncrypted(ctx context.Context, project, scope, key string, valueEncrypted []byte) error {\n\tuid := xid.New().String()\n\t_, err :='),
    ("SetSecretEncrypted INSERT cols",
     'INSERT INTO secrets (project, scope, key, value_encrypted)',
     'INSERT INTO secrets (uid, project, scope, key, value_encrypted)'),
    ("SetSecretEncrypted VALUES",
     "VALUES (?, ?, ?, ?)\n\t\tON CONFLICT",
     "VALUES (?, ?, ?, ?, ?)\n\t\tON CONFLICT"),
    ("SetSecretEncrypted args",
     '`, project, scope, key, valueEncrypted)',
     '`, uid, project, scope, key, valueEncrypted)'),

    # --- Secret SELECTs with value_encrypted (3x) + without (1x) ---
    ("Secret SELECT with value",
     'SELECT id, project, scope, key, value_encrypted, created_at, updated_at',
     'SELECT id, uid, project, scope, key, value_encrypted, created_at, updated_at'),
    ("Secret SELECT without value",
     'SELECT id, project, scope, key, created_at, updated_at',
     'SELECT id, uid, project, scope, key, created_at, updated_at'),

    # --- Secret Scans ---
    ("Secret Scan with ValueEncrypted",
     '&s.ID, &s.Project, &s.Scope, &s.Key, &s.ValueEncrypted, &s.CreatedAt, &s.UpdatedAt',
     '&s.ID, &s.UID, &s.Project, &s.Scope, &s.Key, &s.ValueEncrypted, &s.CreatedAt, &s.UpdatedAt'),
    ("Secret Scan without ValueEncrypted",
     '&s.ID, &s.Project, &s.Scope, &s.Key, &s.CreatedAt, &s.UpdatedAt',
     '&s.ID, &s.UID, &s.Project, &s.Scope, &s.Key, &s.CreatedAt, &s.UpdatedAt'),
])

# ===================================================================
# db_settings.go
# ===================================================================
print("\n=== db_settings.go ===")
process_file(f"{STORAGE}/db_settings.go", [
    # --- Import ---
    ("xid import",
     '"context"\n\t"database/sql"\n\t"fmt"\n)',
     '"context"\n\t"database/sql"\n\t"fmt"\n\n\t"github.com/rs/xid"\n)'),

    # --- SetSetting ---
    ("SetSetting uid gen",
     'func (db *DB) SetSetting(ctx context.Context, category, key, value, valueType string, encrypted bool) error {\n\tencVal :=',
     'func (db *DB) SetSetting(ctx context.Context, category, key, value, valueType string, encrypted bool) error {\n\tuid := xid.New().String()\n\tencVal :='),
    ("SetSetting INSERT cols",
     'INSERT INTO settings (category, key, value, value_type, encrypted)',
     'INSERT INTO settings (uid, category, key, value, value_type, encrypted)'),
    ("SetSetting VALUES",
     "VALUES (?, ?, ?, ?, ?)\n\t\tON CONFLICT",
     "VALUES (?, ?, ?, ?, ?, ?)\n\t\tON CONFLICT"),
    ("SetSetting args",
     '`, category, key, value, valueType, encVal)',
     '`, uid, category, key, value, valueType, encVal)'),

    # --- Setting SELECTs (3x) ---
    ("Setting SELECT cols",
     'SELECT id, category, key, value, value_type, encrypted, description, created_at, updated_at',
     'SELECT id, uid, category, key, value, value_type, encrypted, description, created_at, updated_at'),

    # --- Setting Scans (3x) ---
    ("Setting Scan",
     '&s.ID, &s.Category, &s.Key, &s.Value, &s.ValueType, &encrypted, &description, &s.CreatedAt, &s.UpdatedAt',
     '&s.ID, &s.UID, &s.Category, &s.Key, &s.Value, &s.ValueType, &encrypted, &description, &s.CreatedAt, &s.UpdatedAt'),
])

# ===================================================================
# db_apikeys.go
# ===================================================================
print("\n=== db_apikeys.go ===")
process_file(f"{STORAGE}/db_apikeys.go", [
    # --- Import ---
    ("xid import",
     '\t"go.uber.org/zap"\n)',
     '\t"github.com/rs/xid"\n\t"go.uber.org/zap"\n)'),

    # --- CreateAPIKey ---
    ("CreateAPIKey uid auto-gen",
     'func (db *DB) CreateAPIKey(ctx context.Context, key *APIKey) error {\n\tresult, err :=',
     'func (db *DB) CreateAPIKey(ctx context.Context, key *APIKey) error {\n\tif key.UID == "" {\n\t\tkey.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreateAPIKey INSERT cols",
     'INSERT INTO api_keys (user_id, name, key_hash, key_prefix, scopes, expires_at, created_at)',
     'INSERT INTO api_keys (uid, user_id, name, key_hash, key_prefix, scopes, expires_at, created_at)'),
    ("CreateAPIKey VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?)\n\t`, key.UserID,',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?)\n\t`, key.UID, key.UserID,'),

    # --- API Key SELECTs ---
    ("APIKey SELECT cols",
     'SELECT id, user_id, name, key_hash',
     'SELECT id, uid, user_id, name, key_hash'),

    # --- API Key Scans ---
    ("APIKey Scan",
     '&key.ID, &key.UserID',
     '&key.ID, &key.UID, &key.UserID'),
])

# ===================================================================
# db_health.go
# ===================================================================
print("\n=== db_health.go ===")
process_file(f"{STORAGE}/db_health.go", [
    # --- Import ---
    ("xid import",
     '"context"\n\t"database/sql"\n\t"errors"\n\t"fmt"\n)',
     '"context"\n\t"database/sql"\n\t"errors"\n\t"fmt"\n\n\t"github.com/rs/xid"\n)'),

    # --- CreateHealthCheckConfig ---
    ("CreateHealthCheckConfig uid auto-gen",
     'func (db *DB) CreateHealthCheckConfig(ctx context.Context, config *HealthCheckConfig) error {\n\tresult, err :=',
     'func (db *DB) CreateHealthCheckConfig(ctx context.Context, config *HealthCheckConfig) error {\n\tif config.UID == "" {\n\t\tconfig.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreateHealthCheckConfig INSERT cols",
     'INSERT INTO health_check_configs (project_id, name, url, method,',
     'INSERT INTO health_check_configs (uid, project_id, name, url, method,'),
    ("CreateHealthCheckConfig VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t`, config.ProjectID,',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t`, config.UID, config.ProjectID,'),

    # --- Health SELECTs (4x) ---
    ("Health SELECT cols",
     'SELECT id, project_id, name, url, method, expected_status, timeout_seconds, retries,',
     'SELECT id, uid, project_id, name, url, method, expected_status, timeout_seconds, retries,'),

    # --- Health Scans (4x) ---
    ("Health Scan",
     '&config.ID, &projectID, &config.Name',
     '&config.ID, &config.UID, &projectID, &config.Name'),
    # Also fix GetHealthCheckConfigForProject which uses pid not projectID
    ("Health Scan pid variant",
     '&config.ID, &pid, &config.Name',
     '&config.ID, &config.UID, &pid, &config.Name'),
])

# ===================================================================
# db_security.go (SSH host keys, jump servers, blocked IPs, ACME)
# ===================================================================
print("\n=== db_security.go ===")
process_file(f"{STORAGE}/db_security.go", [
    # --- Import ---
    ("xid import",
     '"context"\n\t"database/sql"\n\t"errors"\n\t"fmt"\n\t"time"\n)',
     '"context"\n\t"database/sql"\n\t"errors"\n\t"fmt"\n\t"time"\n\n\t"github.com/rs/xid"\n)'),

    # --- CreateSSHHostKey ---
    ("CreateSSHHostKey uid auto-gen",
     'func (db *DB) CreateSSHHostKey(ctx context.Context, key *SSHHostKey) error {\n\tresult, err :=',
     'func (db *DB) CreateSSHHostKey(ctx context.Context, key *SSHHostKey) error {\n\tif key.UID == "" {\n\t\tkey.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreateSSHHostKey INSERT cols",
     'INSERT INTO ssh_host_keys (hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at)',
     'INSERT INTO ssh_host_keys (uid, hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at)'),
    ("CreateSSHHostKey VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?)\n\t`, key.Hostname,',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t`, key.UID, key.Hostname,'),

    # --- SSH Host Key SELECTs (3x) ---
    ("SSHHostKey SELECT cols",
     'SELECT id, hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at, created_at, updated_at',
     'SELECT id, uid, hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at, created_at, updated_at'),

    # --- SSH Host Key Scans (3x) ---
    ("SSHHostKey Scan",
     '&key.ID, &key.Hostname, &key.Port, &key.KeyType, &key.PublicKey, &key.Fingerprint',
     '&key.ID, &key.UID, &key.Hostname, &key.Port, &key.KeyType, &key.PublicKey, &key.Fingerprint'),

    # --- CreateJumpServer ---
    ("CreateJumpServer uid auto-gen",
     'func (db *DB) CreateJumpServer(ctx context.Context, js *SSHJumpServer) error {\n\tresult, err :=',
     'func (db *DB) CreateJumpServer(ctx context.Context, js *SSHJumpServer) error {\n\tif js.UID == "" {\n\t\tjs.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreateJumpServer INSERT cols",
     'INSERT INTO ssh_jump_servers (name, host, port, username, ssh_key_id)',
     'INSERT INTO ssh_jump_servers (uid, name, host, port, username, ssh_key_id)'),
    ("CreateJumpServer VALUES",
     'VALUES (?, ?, ?, ?, ?)\n\t`, js.Name,',
     'VALUES (?, ?, ?, ?, ?, ?)\n\t`, js.UID, js.Name,'),

    # --- Jump Server SELECTs (3x) ---
    ("JumpServer SELECT cols",
     'SELECT id, name, host, port, username, ssh_key_id, created_at\n\t\tFROM ssh_jump_servers',
     'SELECT id, uid, name, host, port, username, ssh_key_id, created_at\n\t\tFROM ssh_jump_servers'),

    # --- Jump Server Scans (3x) ---
    ("JumpServer Scan",
     '&js.ID, &js.Name, &js.Host, &js.Port, &js.Username, &sshKeyID, &js.CreatedAt',
     '&js.ID, &js.UID, &js.Name, &js.Host, &js.Port, &js.Username, &sshKeyID, &js.CreatedAt'),

    # --- ACME Certificate SELECTs (2x) ---
    ("ACMECert SELECT cols",
     'SELECT id, domain, certificate_pem, private_key_encrypted, issuer,',
     'SELECT id, uid, domain, certificate_pem, private_key_encrypted, issuer,'),

    # --- ACME Certificate Scans (2x) ---
    ("ACMECert Scan inline",
     '&cert.ID, &cert.Domain, &cert.CertificatePEM, &cert.PrivateKeyEncrypted',
     '&cert.ID, &cert.UID, &cert.Domain, &cert.CertificatePEM, &cert.PrivateKeyEncrypted'),

    # --- SaveACMECertificate ---
    ("SaveACMECert uid auto-gen",
     'func (db *DB) SaveACMECertificate(ctx context.Context, cert *ACMECertificate) error {\n\tresult, err :=',
     'func (db *DB) SaveACMECertificate(ctx context.Context, cert *ACMECertificate) error {\n\tif cert.UID == "" {\n\t\tcert.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("SaveACMECert INSERT cols",
     'INSERT INTO acme_certificates (domain, certificate_pem, private_key_encrypted, issuer,',
     'INSERT INTO acme_certificates (uid, domain, certificate_pem, private_key_encrypted, issuer,'),
    ("SaveACMECert VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)'),
    ("SaveACMECert args",
     '`, cert.Domain, cert.CertificatePEM, cert.PrivateKeyEncrypted, cert.Issuer,',
     '`, cert.UID, cert.Domain, cert.CertificatePEM, cert.PrivateKeyEncrypted, cert.Issuer,'),

    # --- ACME Account SELECTs (1x) ---
    ("ACMEAccount SELECT cols",
     'SELECT id, email, account_url, private_key_encrypted, directory_url, created_at\n\t\tFROM acme_accounts',
     'SELECT id, uid, email, account_url, private_key_encrypted, directory_url, created_at\n\t\tFROM acme_accounts'),

    # --- ACME Account Scans (1x) ---
    ("ACMEAccount Scan",
     '&account.ID, &account.Email, &accountURL, &account.PrivateKeyEncrypted',
     '&account.ID, &account.UID, &account.Email, &accountURL, &account.PrivateKeyEncrypted'),

    # --- SaveACMEAccount (manually managed insert/update) ---
    ("SaveACMEAccount uid auto-gen",
     'if errors.Is(err, sql.ErrNoRows) {\n\t\t// Insert new\n\t\tresult, err := db.conn.ExecContext',
     'if errors.Is(err, sql.ErrNoRows) {\n\t\tif account.UID == "" {\n\t\t\taccount.UID = xid.New().String()\n\t\t}\n\t\t// Insert new\n\t\tresult, err := db.conn.ExecContext'),
    ("SaveACMEAccount INSERT cols",
     'INSERT INTO acme_accounts (email, account_url, private_key_encrypted, directory_url, created_at)',
     'INSERT INTO acme_accounts (uid, email, account_url, private_key_encrypted, directory_url, created_at)'),
    ("SaveACMEAccount VALUES",
     'VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)',
     'VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)'),
    ("SaveACMEAccount args",
     '`, account.Email, account.AccountURL, account.PrivateKeyEncrypted, account.DirectoryURL)',
     '`, account.UID, account.Email, account.AccountURL, account.PrivateKeyEncrypted, account.DirectoryURL)'),
])

# ===================================================================
# memory_security.go (CAs, AgentCerts, SSHKeys, SourceCredentials)
# ===================================================================
print("\n=== memory_security.go ===")
process_file(f"{STORAGE}/memory_security.go", [
    # --- Import ---
    ("xid import",
     '"context"\n\t"database/sql"\n\t"fmt"\n\t"time"\n)',
     '"context"\n\t"database/sql"\n\t"fmt"\n\t"time"\n\n\t"github.com/rs/xid"\n)'),

    # --- CA SELECTs (3x: GetCA, GetCurrentCA, ListCAs) ---
    ("CA SELECT cols",
     'SELECT id, version, common_name, certificate_pem, private_key_encrypted,',
     'SELECT id, uid, version, common_name, certificate_pem, private_key_encrypted,'),

    # --- CA Scans (scanCA + scanCARow = 2x) ---
    ("CA Scan",
     '&ca.ID, &ca.Version, &ca.CommonName, &ca.CertificatePEM, &ca.PrivateKeyEnc',
     '&ca.ID, &ca.UID, &ca.Version, &ca.CommonName, &ca.CertificatePEM, &ca.PrivateKeyEnc'),

    # --- SaveCA (INSERT OR REPLACE) ---
    ("SaveCA uid auto-gen",
     'func (db *DB) SaveCA(ctx context.Context, ca *CertificateAuthority) error {\n\t_, err :=',
     'func (db *DB) SaveCA(ctx context.Context, ca *CertificateAuthority) error {\n\tif ca.UID == "" {\n\t\tca.UID = xid.New().String()\n\t}\n\t_, err :='),
    ("SaveCA INSERT cols",
     '(id, version, common_name, certificate_pem, private_key_encrypted, not_before, not_after, status, is_current, created_at, rotated_at)',
     '(id, uid, version, common_name, certificate_pem, private_key_encrypted, not_before, not_after, status, is_current, created_at, rotated_at)'),
    ("SaveCA VALUES",
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t`, ca.ID,',
     'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t`, ca.ID, ca.UID,'),

    # --- AgentCert SELECTs (4x) ---
    ("AgentCert SELECT cols",
     'SELECT id, agent_id, ca_id, serial_number, certificate_pem, not_before, not_after,',
     'SELECT id, uid, agent_id, ca_id, serial_number, certificate_pem, not_before, not_after,'),

    # --- AgentCert Scans (scanAgentCert + scanAgentCertRow = 2x) ---
    ("AgentCert Scan",
     '&cert.ID, &cert.AgentID, &cert.CAID, &cert.SerialNumber, &cert.CertificatePEM',
     '&cert.ID, &cert.UID, &cert.AgentID, &cert.CAID, &cert.SerialNumber, &cert.CertificatePEM'),

    # --- SaveAgentCert INSERT ---
    ("SaveAgentCert uid auto-gen",
     'if cert.ID == 0 {\n\t\tresult, err := db.conn.ExecContext(ctx, `\n\t\t\tINSERT INTO agent_certificates',
     'if cert.ID == 0 {\n\t\tif cert.UID == "" {\n\t\t\tcert.UID = xid.New().String()\n\t\t}\n\t\tresult, err := db.conn.ExecContext(ctx, `\n\t\t\tINSERT INTO agent_certificates'),
    ("SaveAgentCert INSERT cols",
     '(agent_id, ca_id, serial_number, certificate_pem, not_before, not_after, status, issued_at)',
     '(uid, agent_id, ca_id, serial_number, certificate_pem, not_before, not_after, status, issued_at)'),
    ("SaveAgentCert VALUES",
     "VALUES (?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, cert.AgentID,",
     "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, cert.UID, cert.AgentID,"),

    # --- SSH Key SELECTs (3x) ---
    ("SSHKey SELECT cols",
     'SELECT id, name, public_key, private_key_encrypted, fingerprint, key_type, created_by, created_at\n\t\tFROM ssh_keys',
     'SELECT id, uid, name, public_key, private_key_encrypted, fingerprint, key_type, created_by, created_at\n\t\tFROM ssh_keys'),

    # --- SSH Key Scans (3x) ---
    ("SSHKey Scan",
     '&key.ID, &key.Name, &key.PublicKey, &key.PrivateKeyEnc, &key.Fingerprint',
     '&key.ID, &key.UID, &key.Name, &key.PublicKey, &key.PrivateKeyEnc, &key.Fingerprint'),

    # --- SaveSSHKey INSERT ---
    ("SaveSSHKey uid auto-gen",
     'if key.ID == 0 {\n\t\tif key.CreatedAt.IsZero() {\n\t\t\tkey.CreatedAt = time.Now()\n\t\t}\n\t\tresult, err := db.conn.ExecContext',
     'if key.ID == 0 {\n\t\tif key.UID == "" {\n\t\t\tkey.UID = xid.New().String()\n\t\t}\n\t\tif key.CreatedAt.IsZero() {\n\t\t\tkey.CreatedAt = time.Now()\n\t\t}\n\t\tresult, err := db.conn.ExecContext'),
    ("SaveSSHKey INSERT cols",
     'INSERT INTO ssh_keys (name, public_key, private_key_encrypted, fingerprint, key_type, created_by, created_at)',
     'INSERT INTO ssh_keys (uid, name, public_key, private_key_encrypted, fingerprint, key_type, created_by, created_at)'),
    ("SaveSSHKey VALUES",
     "VALUES (?, ?, ?, ?, ?, ?, ?)\n\t\t`, key.Name,",
     "VALUES (?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, key.UID, key.Name,"),

    # --- SourceCredential SELECTs (3x) ---
    ("SourceCred SELECT cols",
     'SELECT id, name, type, url_pattern, credential_encrypted, created_by, created_at, updated_at\n\t\tFROM source_credentials',
     'SELECT id, uid, name, type, url_pattern, credential_encrypted, created_by, created_at, updated_at\n\t\tFROM source_credentials'),

    # --- SourceCredential Scans (3x) ---
    ("SourceCred Scan",
     '&cred.ID, &cred.Name, &cred.Type, &cred.URLPattern, &cred.CredentialEnc',
     '&cred.ID, &cred.UID, &cred.Name, &cred.Type, &cred.URLPattern, &cred.CredentialEnc'),

    # --- SaveSourceCredential INSERT ---
    ("SaveSourceCred uid auto-gen",
     'if cred.ID == 0 {\n\t\tcred.CreatedAt = now\n\t\tcred.UpdatedAt = now\n\t\tresult, err := db.conn.ExecContext',
     'if cred.ID == 0 {\n\t\tif cred.UID == "" {\n\t\t\tcred.UID = xid.New().String()\n\t\t}\n\t\tcred.CreatedAt = now\n\t\tcred.UpdatedAt = now\n\t\tresult, err := db.conn.ExecContext'),
    ("SaveSourceCred INSERT cols",
     'INSERT INTO source_credentials (name, type, url_pattern, credential_encrypted, created_by, created_at, updated_at)',
     'INSERT INTO source_credentials (uid, name, type, url_pattern, credential_encrypted, created_by, created_at, updated_at)'),
    ("SaveSourceCred VALUES",
     "VALUES (?, ?, ?, ?, ?, ?, ?)\n\t\t`, cred.Name,",
     "VALUES (?, ?, ?, ?, ?, ?, ?, ?)\n\t\t`, cred.UID, cred.Name,"),
])

# ===================================================================
# db_recipes.go
# ===================================================================
print("\n=== db_recipes.go ===")
process_file(f"{STORAGE}/db_recipes.go", [
    # --- Import ---
    ("xid import",
     '"context"\n\t"database/sql"\n\t"encoding/json"\n\t"fmt"\n)',
     '"context"\n\t"database/sql"\n\t"encoding/json"\n\t"fmt"\n\n\t"github.com/rs/xid"\n)'),

    # --- CreateRecipeComponent ---
    ("CreateRecipeComponent uid auto-gen",
     'variablesJSON, err := component.VariablesJSON()\n\tif err != nil {\n\t\treturn fmt.Errorf("marshal variables: %w", err)\n\t}\n\n\tresult, err :=',
     'variablesJSON, err := component.VariablesJSON()\n\tif err != nil {\n\t\treturn fmt.Errorf("marshal variables: %w", err)\n\t}\n\n\tif component.UID == "" {\n\t\tcomponent.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreateRecipeComponent INSERT cols",
     'INSERT INTO recipe_components (\n\t\t\tnamespace, slug, version, name, description, component_type,',
     'INSERT INTO recipe_components (\n\t\t\tuid, namespace, slug, version, name, description, component_type,'),
    ("CreateRecipeComponent VALUES",
     ') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t`, component.Namespace,',
     ') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t`, component.UID, component.Namespace,'),

    # --- Recipe Component SELECTs (shared with playbooks via prefix) ---
    ("RecipeComponent SELECT cols",
     'SELECT id, namespace, slug, version, name, description, component_type,',
     'SELECT id, uid, namespace, slug, version, name, description, component_type,'),

    # --- scanRecipeComponent + scanRecipeComponents Scans (2x) ---
    ("RecipeComponent Scan",
     '&c.ID, &c.Namespace, &c.Slug, &c.Version, &c.Name, &description',
     '&c.ID, &c.UID, &c.Namespace, &c.Slug, &c.Version, &c.Name, &description'),

    # --- CreatePlaybook ---
    ("CreatePlaybook uid auto-gen",
     'validationRulesJSON, err := playbook.ValidationRulesJSON()\n\tif err != nil {\n\t\treturn fmt.Errorf("marshal validation_rules: %w", err)\n\t}\n\n\tresult, err :=',
     'validationRulesJSON, err := playbook.ValidationRulesJSON()\n\tif err != nil {\n\t\treturn fmt.Errorf("marshal validation_rules: %w", err)\n\t}\n\n\tif playbook.UID == "" {\n\t\tplaybook.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreatePlaybook INSERT cols",
     'INSERT INTO playbooks (\n\t\t\tnamespace, slug, version, name, description, framework_type,',
     'INSERT INTO playbooks (\n\t\t\tuid, namespace, slug, version, name, description, framework_type,'),
    ("CreatePlaybook VALUES",
     ') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t`, playbook.Namespace,',
     ') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n\t`, playbook.UID, playbook.Namespace,'),

    # --- Playbook SELECTs (4x) ---
    ("Playbook SELECT cols",
     'SELECT id, namespace, slug, version, name, description, framework_type,',
     'SELECT id, uid, namespace, slug, version, name, description, framework_type,'),

    # --- scanPlaybook + scanPlaybooks Scans (2x) ---
    ("Playbook Scan",
     '&p.ID, &p.Namespace, &p.Slug, &p.Version, &p.Name, &description',
     '&p.ID, &p.UID, &p.Namespace, &p.Slug, &p.Version, &p.Name, &description'),

    # --- CreatePlaybookActivation ---
    ("CreatePlaybookActivation uid auto-gen",
     'func (db *DB) CreatePlaybookActivation(ctx context.Context, activation *PlaybookActivation) error {\n\tresult, err :=',
     'func (db *DB) CreatePlaybookActivation(ctx context.Context, activation *PlaybookActivation) error {\n\tif activation.UID == "" {\n\t\tactivation.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreatePlaybookActivation INSERT cols",
     'INSERT INTO playbook_activations (project_id, playbook_id, activated_by)',
     'INSERT INTO playbook_activations (uid, project_id, playbook_id, activated_by)'),

    # --- Activation SELECTs (3x) ---
    ("Activation SELECT cols",
     'SELECT id, project_id, playbook_id, activated_at, activated_by',
     'SELECT id, uid, project_id, playbook_id, activated_at, activated_by'),

    # --- Activation Scans (scanPlaybookActivation + ListActivationsByPlaybook = 2x) ---
    ("Activation Scan",
     '&a.ID, &a.ProjectID, &a.PlaybookID, &a.ActivatedAt, &activatedBy',
     '&a.ID, &a.UID, &a.ProjectID, &a.PlaybookID, &a.ActivatedAt, &activatedBy'),

    # --- CreateVariableBinding ---
    ("CreateVariableBinding uid auto-gen",
     'func (db *DB) CreateVariableBinding(ctx context.Context, binding *PlaybookVariableBinding) error {\n\tresult, err :=',
     'func (db *DB) CreateVariableBinding(ctx context.Context, binding *PlaybookVariableBinding) error {\n\tif binding.UID == "" {\n\t\tbinding.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreateVariableBinding INSERT cols",
     'INSERT INTO playbook_variable_bindings (\n\t\t\tactivation_id, variable_name, source_type, source_ref, literal_value\n\t\t) VALUES (?, ?, ?, ?, ?)',
     'INSERT INTO playbook_variable_bindings (\n\t\t\tuid, activation_id, variable_name, source_type, source_ref, literal_value\n\t\t) VALUES (?, ?, ?, ?, ?, ?)'),
    ("CreateVariableBinding args",
     '`, binding.ActivationID, binding.VariableName, binding.SourceType,',
     '`, binding.UID, binding.ActivationID, binding.VariableName, binding.SourceType,'),

    # --- Variable Binding SELECTs (2x) ---
    ("VarBinding SELECT cols",
     'SELECT id, activation_id, variable_name, source_type, source_ref, literal_value',
     'SELECT id, uid, activation_id, variable_name, source_type, source_ref, literal_value'),

    # --- Variable Binding Scans ---
    ("VarBinding Scan",
     '&b.ID, &b.ActivationID, &b.VariableName, &b.SourceType, &sourceRef, &literalValue',
     '&b.ID, &b.UID, &b.ActivationID, &b.VariableName, &b.SourceType, &sourceRef, &literalValue'),

    # --- CreateRawApproval ---
    ("CreateRawApproval uid auto-gen",
     'func (db *DB) CreateRawApproval(ctx context.Context, approval *RawCommandApproval) error {\n\tresult, err :=',
     'func (db *DB) CreateRawApproval(ctx context.Context, approval *RawCommandApproval) error {\n\tif approval.UID == "" {\n\t\tapproval.UID = xid.New().String()\n\t}\n\tresult, err :='),
    ("CreateRawApproval INSERT cols",
     'INSERT INTO raw_command_approvals (component_id, approved_by, approval_note)',
     'INSERT INTO raw_command_approvals (uid, component_id, approved_by, approval_note)'),

    # --- Raw Approval SELECTs (2x) ---
    ("RawApproval SELECT cols",
     'SELECT id, component_id, approved_by, approved_at, approval_note',
     'SELECT id, uid, component_id, approved_by, approved_at, approval_note'),

    # --- Raw Approval Scans (2x) ---
    ("RawApproval Scan",
     '&a.ID, &a.ComponentID, &a.ApprovedBy, &a.ApprovedAt, &approvalNote',
     '&a.ID, &a.UID, &a.ComponentID, &a.ApprovedBy, &a.ApprovedAt, &approvalNote'),

    # --- Both Activation and RawApproval use (?, ?, ?) VALUES (handle together: 2x 3->4) ---
    # After the INSERT cols are changed, the VALUES still need updating
    # CreatePlaybookActivation: VALUES (?, ?, ?) with activation.ProjectID
    ("Activation VALUES",
     "VALUES (?, ?, ?, ?)\n\t`, activation.ProjectID,",
     "VALUES (?, ?, ?, ?, ?)\n\t`, activation.UID, activation.ProjectID,"),
    # CreateRawApproval: VALUES (?, ?, ?) with approval.ComponentID 
    ("RawApproval VALUES",
     "VALUES (?, ?, ?, ?)\n\t`, approval.ComponentID,",
     "VALUES (?, ?, ?, ?, ?)\n\t`, approval.UID, approval.ComponentID,"),
])

print("\n=== DONE ===")
print("Run 'go build ./...' to verify.")
