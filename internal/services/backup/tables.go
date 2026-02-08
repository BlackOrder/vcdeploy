// Package backup provides export and import services for vcdeploy backups.
package backup

// TableExportConfig defines how a table is handled during export/import.
type TableExportConfig struct {
	Name             string            // Table name in the database
	EncryptedColumns map[string]string // Column name → encryption type ("kms" or "masterkey")
	DependsOn        []string          // Tables that must be exported/imported first (for FK ordering)
}

// ExportableTables lists all tables that are included in exports, ordered by FK dependencies.
// Tables NOT in this list are never exported (sessions, encryption_keys, master_key_meta,
// schema_migrations, rate_limits, deployment_logs, audit_logs, agents, deployments, etc.).
var ExportableTables = []TableExportConfig{
	// --- Always Importable (no encrypted columns) ---
	{
		Name: "project_types",
	},
	{
		Name:      "projects",
		DependsOn: []string{"project_types"},
	},
	{
		Name:      "health_check_configs",
		DependsOn: []string{"projects"},
	},
	{
		Name: "known_hosts",
	},
	{
		Name: "ssh_host_keys",
	},
	{
		Name: "recipe_components",
	},
	{
		Name:      "playbooks",
		DependsOn: []string{"recipe_components"},
	},
	{
		Name:      "playbook_activations",
		DependsOn: []string{"projects", "playbooks"},
	},
	{
		Name:      "playbook_variable_bindings",
		DependsOn: []string{"playbook_activations"},
	},
	{
		Name:      "raw_command_approvals",
		DependsOn: []string{"recipe_components", "users"},
	},

	// --- Selectively Importable (contain portable hashes or encrypted data) ---
	{
		Name: "users",
		// password_hash is bcrypt — portable, not encrypted
	},
	{
		Name:      "api_keys",
		DependsOn: []string{"users"},
		// key_hash is a hash — portable, not encrypted
	},
	{
		Name: "ssh_keys",
		EncryptedColumns: map[string]string{
			"private_key_encrypted": "masterkey",
		},
	},
	{
		Name:      "ssh_jump_servers",
		DependsOn: []string{"ssh_keys"},
	},
	{
		Name:      "secrets",
		DependsOn: []string{"projects"},
		EncryptedColumns: map[string]string{
			"value_encrypted": "kms",
		},
	},
	{
		Name: "source_credentials",
		EncryptedColumns: map[string]string{
			"credential_encrypted": "kms",
		},
	},
	{
		Name:      "project_webhooks",
		DependsOn: []string{"projects"},
		EncryptedColumns: map[string]string{
			"secret_encrypted": "kms",
		},
	},
	{
		Name: "settings",
		// Settings with encrypted=1 have KMS-encrypted values in the "value" column.
		// Handled specially during export: check the encrypted flag per row.
	},
	{
		Name: "webhook_secrets",
		EncryptedColumns: map[string]string{
			"secret_encrypted": "kms",
		},
	},
	{
		Name: "certificate_authorities",
		EncryptedColumns: map[string]string{
			"private_key_encrypted": "masterkey",
		},
	},
	{
		Name:      "agent_certificates",
		DependsOn: []string{"certificate_authorities"},
		EncryptedColumns: map[string]string{
			// agent_certificates reference agents (never exported) via agent_id,
			// but the private_key is masterkey-encrypted
			"private_key_encrypted": "masterkey",
		},
	},
	{
		Name: "acme_certificates",
		EncryptedColumns: map[string]string{
			"private_key_encrypted": "masterkey",
		},
	},
	{
		Name: "acme_accounts",
		EncryptedColumns: map[string]string{
			"private_key_encrypted": "masterkey",
		},
	},
}

// ExportableTableNames returns just the table names for quick lookup.
func ExportableTableNames() []string {
	names := make([]string, len(ExportableTables))
	for i, t := range ExportableTables {
		names[i] = t.Name
	}
	return names
}

// IsExportableTable returns true if the named table is in the export list.
func IsExportableTable(name string) bool {
	for _, t := range ExportableTables {
		if t.Name == name {
			return true
		}
	}
	return false
}

// GetTableConfig returns the export config for a table, or nil if not exportable.
func GetTableConfig(name string) *TableExportConfig {
	for i := range ExportableTables {
		if ExportableTables[i].Name == name {
			return &ExportableTables[i]
		}
	}
	return nil
}
