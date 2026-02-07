package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// LoadFromDB loads all data from a SQLite DB into the MemoryStore.
// This is used during startup to populate the memory store from persistent storage.
// The provided DB should be the main application database.
func (s *MemoryStore) LoadFromDB(ctx context.Context, db *DB) error {
	start := time.Now()

	// Load in dependency order - entities with no dependencies first
	loaders := []struct {
		name string
		fn   func(context.Context, *DB) error
	}{
		{"users", s.loadUsers},
		{"sessions", s.loadSessions},
		{"api_keys", s.loadAPIKeys},
		{"settings", s.loadSettings},
		{"project_types", s.loadProjectTypes},
		{"projects", s.loadProjects},
		{"webhooks", s.loadWebhooks},
		{"secrets", s.loadSecrets},
		{"agents", s.loadAgents},
		{"agent_binaries", s.loadAgentBinaries},
		{"deployments", s.loadDeployments},
		{"deployment_logs", s.loadDeploymentLogs},
		{"deployment_rollbacks", s.loadDeploymentRollbacks},
		{"scheduled_deployments", s.loadScheduledDeployments},
		{"audit_logs", s.loadAuditLogs},
		{"blocked_ips", s.loadBlockedIPs},
		{"rate_limits", s.loadRateLimits},
		{"provision_jobs", s.loadProvisionJobs},
		{"ssh_host_keys", s.loadSSHHostKeys},
		{"jump_servers", s.loadJumpServers},
		{"health_check_configs", s.loadHealthCheckConfigs},
		// Security tables
		{"certificate_authorities", s.loadCertificateAuthorities},
		{"agent_certificates", s.loadAgentCertificates},
		{"server_certificates", s.loadServerCertificates},
		{"registration_tokens", s.loadRegistrationTokens},
		{"source_credentials", s.loadSourceCredentials},
		{"revoked_certificates", s.loadRevokedCertificates},
		{"encryption_keys", s.loadEncryptionKeys},
		{"ssh_keys", s.loadSSHKeys},
		{"cert_audit_events", s.loadCertAuditEvents},
		// ACME tables
		{"acme_certificates", s.loadACMECertificates},
		{"acme_accounts", s.loadACMEAccounts},
		// Recovery codes
		{"recovery_codes", s.loadRecoveryCodes},
		// Recipe system tables
		{"recipe_components", s.loadRecipeComponentsFromDB},
		{"playbooks", s.loadPlaybooksFromDB},
		{"playbook_activations", s.loadPlaybookActivationsFromDB},
		{"playbook_variable_bindings", s.loadVariableBindingsFromDB},
		{"raw_command_approvals", s.loadRawApprovalsFromDB},
	}

	for _, loader := range loaders {
		if err := loader.fn(ctx, db); err != nil {
			return fmt.Errorf("loading %s: %w", loader.name, err)
		}
	}

	s.logger.Info("loaded data from DB",
		zap.Duration("elapsed", time.Since(start)))

	return nil
}

func (s *MemoryStore) loadUsers(ctx context.Context, db *DB) error {
	users, err := db.ListUsers(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, u := range users {
		stored := *u
		s.users[u.ID] = &stored
		s.usersByName[u.Username] = &stored
		if u.ID >= s.nextUserID.Load() {
			s.nextUserID.Store(u.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadSessions(ctx context.Context, db *DB) error {
	// Load sessions for all users
	s.mu.RLock()
	userIDs := make([]int64, 0, len(s.users))
	for id := range s.users {
		userIDs = append(userIDs, id)
	}
	s.mu.RUnlock()

	for _, userID := range userIDs {
		sessions, err := db.ListUserSessions(ctx, userID)
		if err != nil {
			return err
		}

		s.mu.Lock()
		for _, sess := range sessions {
			stored := *sess
			s.sessions[sess.Token] = &stored
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *MemoryStore) loadAPIKeys(ctx context.Context, db *DB) error {
	s.mu.RLock()
	userIDs := make([]int64, 0, len(s.users))
	for id := range s.users {
		userIDs = append(userIDs, id)
	}
	s.mu.RUnlock()

	for _, userID := range userIDs {
		keys, err := db.ListAPIKeys(ctx, userID)
		if err != nil {
			return err
		}

		s.mu.Lock()
		for _, key := range keys {
			stored := *key
			s.apiKeys[key.KeyHash] = &stored
			s.apiKeysByUser[userID] = append(s.apiKeysByUser[userID], &stored)
			if key.ID >= s.nextAPIKeyID.Load() {
				s.nextAPIKeyID.Store(key.ID + 1)
			}
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *MemoryStore) loadSettings(ctx context.Context, db *DB) error {
	settings, err := db.ListAllSettings(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, setting := range settings {
		stored := *setting
		key := settingKey(setting.Category, setting.Key)
		s.settings[key] = &stored
		if setting.ID >= s.nextSettingID.Load() {
			s.nextSettingID.Store(setting.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadProjectTypes(ctx context.Context, db *DB) error {
	types, err := db.ListProjectTypes(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pt := range types {
		stored := *pt
		s.projectTypes[pt.ID] = &stored
		if pt.ID >= s.nextProjectTypeID.Load() {
			s.nextProjectTypeID.Store(pt.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadProjects(ctx context.Context, db *DB) error {
	projects, err := db.ListProjects(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range projects {
		stored := *p
		s.projects[p.ID] = &stored
		s.projectsByName[p.Name] = &stored
		if p.ID >= s.nextProjectID.Load() {
			s.nextProjectID.Store(p.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadWebhooks(ctx context.Context, db *DB) error {
	s.mu.RLock()
	projectIDs := make([]int64, 0, len(s.projects))
	for id := range s.projects {
		projectIDs = append(projectIDs, id)
	}
	s.mu.RUnlock()

	for _, projectID := range projectIDs {
		webhooks, err := db.ListProjectWebhooks(ctx, projectID)
		if err != nil {
			return err
		}

		s.mu.Lock()
		for _, wh := range webhooks {
			stored := *wh
			s.webhooks[wh.ID] = &stored
			if wh.ID >= s.nextWebhookID.Load() {
				s.nextWebhookID.Store(wh.ID + 1)
			}
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *MemoryStore) loadSecrets(ctx context.Context, db *DB) error {
	secrets, err := db.ListAllSecretsCtx(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, secret := range secrets {
		stored := *secret
		key := secretKey(secret.Project, secret.Scope, secret.Key)
		s.secrets[key] = &stored
		if secret.ID >= s.nextSecretID.Load() {
			s.nextSecretID.Store(secret.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadAgents(ctx context.Context, db *DB) error {
	agents, err := db.ListAgents(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, agent := range agents {
		stored := *agent
		// Deep copy labels map
		if agent.Labels != nil {
			stored.Labels = make(map[string]string, len(agent.Labels))
			for k, v := range agent.Labels {
				stored.Labels[k] = v
			}
		}
		s.agents[agent.ID] = &stored
	}
	return nil
}

func (s *MemoryStore) loadAgentBinaries(ctx context.Context, db *DB) error {
	binaries, err := db.ListAgentBinaries(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, bin := range binaries {
		stored := *bin
		s.agentBinaries[bin.ID] = &stored
		if bin.ID >= s.nextAgentBinaryID.Load() {
			s.nextAgentBinaryID.Store(bin.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadDeployments(ctx context.Context, db *DB) error {
	// Load recent deployments (last 10000 to avoid loading entire history)
	deployments, err := db.ListDeploymentsRecent(ctx, 10000)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, dep := range deployments {
		stored := *dep
		s.deployments[dep.ID] = &stored
	}
	return nil
}

func (s *MemoryStore) loadDeploymentLogs(ctx context.Context, db *DB) error {
	// Load logs for deployments we have in memory
	s.mu.RLock()
	deploymentIDs := make([]string, 0, len(s.deployments))
	for id := range s.deployments {
		deploymentIDs = append(deploymentIDs, id)
	}
	s.mu.RUnlock()

	for _, depID := range deploymentIDs {
		logs, err := db.ListDeploymentLogs(ctx, depID)
		if err != nil {
			return err
		}

		s.mu.Lock()
		for _, log := range logs {
			stored := *log
			s.deploymentLogs[depID] = append(s.deploymentLogs[depID], &stored)
			if log.ID >= s.nextDeploymentLogID.Load() {
				s.nextDeploymentLogID.Store(log.ID + 1)
			}
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *MemoryStore) loadDeploymentRollbacks(ctx context.Context, db *DB) error {
	// Load rollbacks for deployments we have in memory
	// Get rollbacks by querying each deployment
	s.mu.RLock()
	deploymentIDs := make([]string, 0, len(s.deployments))
	for id := range s.deployments {
		deploymentIDs = append(deploymentIDs, id)
	}
	s.mu.RUnlock()

	for _, depID := range deploymentIDs {
		rollback, err := db.GetLatestRollbackForDeployment(ctx, depID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}

		s.mu.Lock()
		stored := *rollback
		s.deploymentRollbacks[rollback.ID] = &stored
		if rollback.ID >= s.nextRollbackID.Load() {
			s.nextRollbackID.Store(rollback.ID + 1)
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *MemoryStore) loadScheduledDeployments(ctx context.Context, db *DB) error {
	scheduled, err := db.ListPendingScheduledDeployments(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sd := range scheduled {
		stored := *sd
		s.scheduledDeploys[sd.ID] = &stored
	}
	return nil
}

func (s *MemoryStore) loadAuditLogs(ctx context.Context, db *DB) error {
	// Load recent audit logs (last 10000)
	logs, err := db.ListAuditLogs(ctx, 10000, 0)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, log := range logs {
		stored := *log
		s.auditLogs = append(s.auditLogs, &stored)
		if log.ID >= s.nextAuditID.Load() {
			s.nextAuditID.Store(log.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadBlockedIPs(ctx context.Context, db *DB) error {
	// Load all blocked IPs (use large limit)
	blocked, _, err := db.ListBlockedIPs(ctx, 100000, 0)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, b := range blocked {
		stored := *b
		s.blockedIPs[b.IPAddress] = &stored
		if b.ID >= s.nextBlockedIPID.Load() {
			s.nextBlockedIPID.Store(b.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadRateLimits(ctx context.Context, db *DB) error {
	// Rate limits are typically ephemeral, skip loading old ones
	// They will be recreated as requests come in
	return nil
}

func (s *MemoryStore) loadProvisionJobs(ctx context.Context, db *DB) error {
	// Load pending provision jobs
	jobs, err := db.ListPendingProvisionJobs(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, job := range jobs {
		stored := *job
		s.provisionJobs[job.ID] = &stored
	}
	return nil
}

func (s *MemoryStore) loadSSHHostKeys(ctx context.Context, db *DB) error {
	keys, err := db.ListSSHHostKeys(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		stored := *key
		s.sshHostKeys[key.ID] = &stored
		if key.ID >= s.nextSSHHostKeyID.Load() {
			s.nextSSHHostKeyID.Store(key.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadJumpServers(ctx context.Context, db *DB) error {
	servers, err := db.ListJumpServers(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, srv := range servers {
		stored := *srv
		s.jumpServers[srv.ID] = &stored
		if srv.ID >= s.nextJumpServerID.Load() {
			s.nextJumpServerID.Store(srv.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadHealthCheckConfigs(ctx context.Context, db *DB) error {
	configs, err := db.ListHealthCheckConfigs(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cfg := range configs {
		stored := *cfg
		s.healthCheckConfigs[cfg.ID] = &stored
		if cfg.ID >= s.nextHealthCheckID.Load() {
			s.nextHealthCheckID.Store(cfg.ID + 1)
		}
	}
	return nil
}

// --- Security table loaders ---

func (s *MemoryStore) loadCertificateAuthorities(ctx context.Context, db *DB) error {
	cas, err := db.ListCAs(ctx)
	if err != nil {
		// Table might not exist yet during migration
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ca := range cas {
		stored := *ca
		s.certificateAuthorities[ca.ID] = &stored
	}
	return nil
}

func (s *MemoryStore) loadAgentCertificates(ctx context.Context, db *DB) error {
	certs, err := db.ListAgentCerts(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cert := range certs {
		stored := *cert
		s.agentCertificates[cert.SerialNumber] = &stored
		if cert.ID >= s.nextAgentCertID.Load() {
			s.nextAgentCertID.Store(cert.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadServerCertificates(ctx context.Context, db *DB) error {
	certs, err := db.ListServerCerts(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cert := range certs {
		stored := *cert
		s.serverCertificates[cert.Hostname] = &stored
		if cert.ID >= s.nextServerCertID.Load() {
			s.nextServerCertID.Store(cert.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadRegistrationTokens(ctx context.Context, db *DB) error {
	tokens, err := db.ListRegistrationTokens(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, token := range tokens {
		stored := *token
		s.registrationTokens[token.Token] = &stored
		if token.ID >= s.nextRegTokenID.Load() {
			s.nextRegTokenID.Store(token.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadSourceCredentials(ctx context.Context, db *DB) error {
	creds, err := db.ListSourceCredentials(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cred := range creds {
		stored := *cred
		s.sourceCredentials[cred.ID] = &stored
		if cred.ID >= s.nextSourceCredID.Load() {
			s.nextSourceCredID.Store(cred.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadRevokedCertificates(ctx context.Context, db *DB) error {
	certs, err := db.ListRevokedCerts(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cert := range certs {
		stored := *cert
		s.revokedCertificates[cert.SerialNumber] = &stored
		if cert.ID >= s.nextRevokedCertID.Load() {
			s.nextRevokedCertID.Store(cert.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadEncryptionKeys(ctx context.Context, db *DB) error {
	keys, err := db.ListEncryptionKeys(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		stored := *key
		s.encryptionKeys[key.ID] = &stored
	}
	return nil
}

func (s *MemoryStore) loadSSHKeys(ctx context.Context, db *DB) error {
	keys, err := db.ListSSHKeys(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		stored := *key
		s.sshKeys[key.ID] = &stored
		if key.ID >= s.nextSSHKeyID.Load() {
			s.nextSSHKeyID.Store(key.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadCertAuditEvents(ctx context.Context, db *DB) error {
	// Load limited audit history (last 30 days)
	filter := CertAuditFilter{
		Limit: 10000, // Cap at 10k events in memory
	}

	events, err := db.ListCertAuditEvents(ctx, filter)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range events {
		stored := *event
		s.certAuditEvents = append(s.certAuditEvents, &stored)
		if event.ID >= s.nextCertAuditID.Load() {
			s.nextCertAuditID.Store(event.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadACMECertificates(ctx context.Context, db *DB) error {
	certs, err := db.ListACMECertificates(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cert := range certs {
		stored := *cert
		s.acmeCertificates[cert.Domain] = &stored
		if cert.ID >= s.nextACMECertID.Load() {
			s.nextACMECertID.Store(cert.ID + 1)
		}
	}
	return nil
}

func (s *MemoryStore) loadACMEAccounts(ctx context.Context, db *DB) error {
	// ACME accounts are loaded on-demand via email lookup
	// We query all accounts from DB
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, uid, email, account_url, private_key_encrypted, directory_url, created_at
		FROM acme_accounts ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for rows.Next() {
		var account ACMEAccount
		var accountURL string
		if err := rows.Scan(&account.ID, &account.UID, &account.Email, &accountURL,
			&account.PrivateKeyEncrypted, &account.DirectoryURL, &account.CreatedAt); err != nil {
			return err
		}
		account.AccountURL = accountURL
		s.acmeAccounts[account.Email] = &account
		if account.ID >= s.nextACMEAccountID.Load() {
			s.nextACMEAccountID.Store(account.ID + 1)
		}
	}
	return rows.Err()
}

func (s *MemoryStore) loadRecoveryCodes(ctx context.Context, db *DB) error {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, user_id, code_hash, used_at, created_at
		FROM recovery_codes ORDER BY user_id, id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for rows.Next() {
		var code RecoveryCode
		if err := rows.Scan(&code.ID, &code.UserID, &code.CodeHash, &code.UsedAt, &code.CreatedAt); err != nil {
			return err
		}
		s.recoveryCodes[code.UserID] = append(s.recoveryCodes[code.UserID], &code)
		if code.ID >= s.nextRecoveryCodeID.Load() {
			s.nextRecoveryCodeID.Store(code.ID + 1)
		}
	}
	return rows.Err()
}
