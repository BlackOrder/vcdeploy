package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// certCmd handles certificate management commands
var certCmd = &cobra.Command{
	Use:   "cert",
	Short: "Certificate management",
	Long: `Commands for managing TLS certificates.

This includes agent certificates, CA certificates, and certificate audit logs.
All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(certCmd)

	// List certificates
	certCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all agent certificates",
		Long: `List all agent certificates with their status.

Example:
  vcdeploy certs list --master localhost:9000 --token <token>`,
		RunE: runCertsList,
	})

	// Show certificate details
	showCertCmd := &cobra.Command{
		Use:   "show [agent-id]",
		Short: "Show certificate details for an agent",
		Long: `Display detailed information about an agent's certificate.

Example:
  vcdeploy certs show agent-001 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runCertsShow,
	}
	certCmd.AddCommand(showCertCmd)

	// Revoke certificate
	revokeCertCmd := &cobra.Command{
		Use:   "revoke [agent-id]",
		Short: "Revoke an agent's certificate",
		Long: `Revoke an agent's certificate, preventing it from authenticating.

Example:
  vcdeploy certs revoke agent-001 --reason "Agent compromised" --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runCertsRevoke,
	}
	revokeCertCmd.Flags().StringP("reason", "r", "", "Reason for revocation")
	certCmd.AddCommand(revokeCertCmd)

	// CA subcommand
	caCmd := &cobra.Command{
		Use:   "ca",
		Short: "Certificate Authority management",
		Long:  "Commands for managing the Certificate Authority.",
	}
	certCmd.AddCommand(caCmd)

	caCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List CA certificates",
		Long: `List all CA certificates with their validity periods.

Example:
  vcdeploy certs ca list --master localhost:9000 --token <token>`,
		RunE: runCAList,
	})

	rotateCACmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate the CA certificate",
		Long: `Generate a new CA certificate and begin using it for new agent certificates.

This operation:
1. Creates a new CA with configurable validity period
2. Keeps the old CA for verifying existing agent certificates
3. New agent certificates will use the new CA

Example:
  vcdeploy certs ca rotate --validity-days 365 --master localhost:9000 --token <token>`,
		RunE: runCARotate,
	}
	rotateCACmd.Flags().IntP("validity-days", "d", 365, "CA validity period in days")
	caCmd.AddCommand(rotateCACmd)

	// Audit command
	auditCmd := &cobra.Command{
		Use:   "audit",
		Short: "View certificate audit log",
		Long: `Display the certificate audit log showing all certificate operations.

Example:
  vcdeploy certs audit --limit 50 --master localhost:9000 --token <token>`,
		RunE: runCertsAudit,
	}
	auditCmd.Flags().IntP("limit", "n", 100, "Maximum number of entries to show")
	auditCmd.Flags().String("agent", "", "Filter by agent ID")
	auditCmd.Flags().String("action", "", "Filter by action (issue, revoke, renew)")
	certCmd.AddCommand(auditCmd)
}

// --- Certificate List ---

type agentCertResponse struct {
	Certificates []agentCertInfo `json:"certificates"`
}

type agentCertInfo struct {
	AgentID       string    `json:"agent_id"`
	SerialNumber  string    `json:"serial_number"`
	Fingerprint   string    `json:"fingerprint"`
	IssuedAt      time.Time `json:"issued_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	Status        string    `json:"status"`
	ExpiresIn     string    `json:"expires_in,omitempty"`
	CAFingerprint string    `json:"ca_fingerprint,omitempty"`
}

func runCertsList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/certificates/agents")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result agentCertResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Certificates) == 0 {
		fmt.Println("No agent certificates found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT ID\tSTATUS\tISSUED\tEXPIRES\tEXPIRES IN")

	for i := range result.Certificates {
		cert := &result.Certificates[i]
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			cert.AgentID,
			cert.Status,
			cert.IssuedAt.Format("2006-01-02"),
			cert.ExpiresAt.Format("2006-01-02"),
			cert.ExpiresIn,
		)
	}
	_ = w.Flush() // #nosec G104 - best effort output flush

	return nil
}

// --- Certificate Show ---

func runCertsShow(cmd *cobra.Command, args []string) error {
	agentID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/certificates/agents/" + agentID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var cert agentCertInfo
	if err := json.NewDecoder(resp.Body).Decode(&cert); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Agent ID:       %s\n", cert.AgentID)
	fmt.Printf("Status:         %s\n", cert.Status)
	fmt.Printf("Serial Number:  %s\n", cert.SerialNumber)
	fmt.Printf("Fingerprint:    %s\n", cert.Fingerprint)
	fmt.Printf("Issued At:      %s\n", cert.IssuedAt.Format(time.RFC3339))
	fmt.Printf("Expires At:     %s\n", cert.ExpiresAt.Format(time.RFC3339))
	fmt.Printf("Expires In:     %s\n", cert.ExpiresIn)
	if cert.CAFingerprint != "" {
		fmt.Printf("CA Fingerprint: %s\n", cert.CAFingerprint)
	}

	return nil
}

// --- Certificate Revoke ---

func runCertsRevoke(cmd *cobra.Command, args []string) error {
	agentID := args[0]
	reason, _ := cmd.Flags().GetString("reason")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]string{}
	if reason != "" {
		reqBody["reason"] = reason
	}

	body, _ := json.Marshal(reqBody)
	resp, err := client.post("/api/v1/certificates/agents/"+agentID+"/revoke", jsonReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return handleAPIError(resp)
	}

	fmt.Printf("Certificate for agent %s has been revoked.\n", agentID)
	return nil
}

// --- CA List ---

type caListResponse struct {
	CAs []caInfo `json:"cas"`
}

type caInfo struct {
	Fingerprint string    `json:"fingerprint"`
	Subject     string    `json:"subject"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	IsCurrent   bool      `json:"is_current"`
	ExpiresIn   string    `json:"expires_in,omitempty"`
}

func runCAList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/certificates/cas")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result caListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.CAs) == 0 {
		fmt.Println("No CA certificates found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FINGERPRINT\tSUBJECT\tCURRENT\tEXPIRES\tEXPIRES IN")

	for _, ca := range result.CAs {
		current := ""
		if ca.IsCurrent {
			current = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			truncate(ca.Fingerprint, 16),
			ca.Subject,
			current,
			ca.ExpiresAt.Format("2006-01-02"),
			ca.ExpiresIn,
		)
	}
	_ = w.Flush() // #nosec G104 - best effort output flush

	return nil
}

// --- CA Rotate ---

func runCARotate(cmd *cobra.Command, args []string) error {
	validityDays, _ := cmd.Flags().GetInt("validity-days")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{
		"validity_days": validityDays,
	}
	body, _ := json.Marshal(reqBody)

	fmt.Println("Rotating CA certificate...")
	resp, err := client.post("/api/v1/certificates/cas/rotate", jsonReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return handleAPIError(resp)
	}

	var newCA caInfo
	if err := json.NewDecoder(resp.Body).Decode(&newCA); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Println("CA certificate rotated successfully.")
	fmt.Printf("New CA Fingerprint: %s\n", newCA.Fingerprint)
	fmt.Printf("Validity: %d days (expires %s)\n", validityDays, newCA.ExpiresAt.Format("2006-01-02"))

	return nil
}

// --- Audit Log ---

type auditResponse struct {
	Events []auditEvent `json:"events"`
}

type auditEvent struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Action      string    `json:"action"`
	AgentID     string    `json:"agent_id,omitempty"`
	PerformedBy string    `json:"performed_by,omitempty"`
	Details     string    `json:"details,omitempty"`
}

func runCertsAudit(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	agentFilter, _ := cmd.Flags().GetString("agent")
	actionFilter, _ := cmd.Flags().GetString("action")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// Build query params
	path := fmt.Sprintf("/api/v1/certificates/audit?limit=%d", limit)
	if agentFilter != "" {
		path += "&agent=" + agentFilter
	}
	if actionFilter != "" {
		path += "&action=" + actionFilter
	}

	resp, err := client.get(path)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result auditResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Events) == 0 {
		fmt.Println("No audit events found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tACTION\tAGENT\tPERFORMED BY\tDETAILS")

	for _, event := range result.Events {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			event.Timestamp.Format("2006-01-02 15:04"),
			event.Action,
			event.AgentID,
			event.PerformedBy,
			truncate(event.Details, 40),
		)
	}
	_ = w.Flush() // #nosec G104 - best effort output flush

	return nil
}

// --- TLS subcommand ---

func init() {
	// TLS subcommand for server TLS configuration
	tlsCmd := &cobra.Command{
		Use:   "tls",
		Short: "Server TLS configuration",
		Long:  "Commands for managing server TLS settings and certificates.",
	}
	certCmd.AddCommand(tlsCmd)

	// TLS status
	tlsCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show TLS certificate status",
		Long: `Display status of the server's TLS certificate.

Example:
  vcdeploy certs tls status --master localhost:9000 --token <token>`,
		RunE: runTLSStatus,
	})

	// TLS renew
	tlsRenewCmd := &cobra.Command{
		Use:   "renew",
		Short: "Renew TLS certificate",
		Long: `Renew the server's TLS certificate (ACME/Let's Encrypt).

Example:
  vcdeploy certs tls renew --master localhost:9000 --token <token>`,
		RunE: runTLSRenew,
	}
	tlsRenewCmd.Flags().BoolP("force", "f", false, "Force renewal even if not near expiry")
	tlsCmd.AddCommand(tlsRenewCmd)

	// TLS settings
	tlsSettingsCmd := &cobra.Command{
		Use:   "settings",
		Short: "Manage TLS settings",
		Long: `View or update TLS settings.

Example:
  vcdeploy certs tls settings --master localhost:9000 --token <token>
  vcdeploy certs tls settings --min-version TLS12 --master localhost:9000 --token <token>`,
		RunE: runTLSSettings,
	}
	tlsSettingsCmd.Flags().String("min-version", "", "Minimum TLS version (TLS10, TLS11, TLS12, TLS13)")
	tlsSettingsCmd.Flags().StringSlice("ciphers", nil, "Allowed cipher suites")
	tlsCmd.AddCommand(tlsSettingsCmd)
}

type tlsStatusResponse struct {
	Enabled      bool   `json:"enabled"`
	Certificate  string `json:"certificate"`
	Issuer       string `json:"issuer"`
	Subject      string `json:"subject"`
	NotBefore    string `json:"not_before"`
	NotAfter     string `json:"not_after"`
	SerialNumber string `json:"serial_number"`
	Fingerprint  string `json:"fingerprint"`
	ACMEEnabled  bool   `json:"acme_enabled"`
	ACMEDomain   string `json:"acme_domain,omitempty"`
	AutoRenew    bool   `json:"auto_renew"`
	DaysUntilExp int    `json:"days_until_expiry"`
}

func runTLSStatus(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/tls/status")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var status tlsStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("TLS Enabled:     %v\n", status.Enabled)
	if !status.Enabled {
		return nil
	}

	fmt.Printf("Subject:         %s\n", status.Subject)
	fmt.Printf("Issuer:          %s\n", status.Issuer)
	fmt.Printf("Valid From:      %s\n", status.NotBefore)
	fmt.Printf("Valid Until:     %s\n", status.NotAfter)
	fmt.Printf("Days Until Exp:  %d\n", status.DaysUntilExp)
	fmt.Printf("Serial Number:   %s\n", status.SerialNumber)
	fmt.Printf("Fingerprint:     %s\n", status.Fingerprint)
	fmt.Printf("ACME Enabled:    %v\n", status.ACMEEnabled)
	if status.ACMEEnabled {
		fmt.Printf("ACME Domain:     %s\n", status.ACMEDomain)
	}
	fmt.Printf("Auto Renew:      %v\n", status.AutoRenew)

	// Warning if expiring soon
	if status.DaysUntilExp <= 30 {
		fmt.Printf("\n⚠️  Certificate expires in %d days!\n", status.DaysUntilExp)
	}

	return nil
}

func runTLSRenew(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	endpoint := "/api/v1/tls/renew"
	if force {
		endpoint += "?force=true"
	}

	resp, err := client.post(endpoint, nil)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result struct {
		Success     bool   `json:"success"`
		Message     string `json:"message"`
		Certificate string `json:"certificate,omitempty"`
		NotAfter    string `json:"not_after,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if result.Success {
		fmt.Println("✅ TLS certificate renewed successfully.")
		if result.NotAfter != "" {
			fmt.Printf("   New expiry: %s\n", result.NotAfter)
		}
	} else {
		fmt.Printf("❌ Renewal failed: %s\n", result.Message)
	}

	return nil
}

type tlsSettingsResponse struct {
	MinVersion   string   `json:"min_version"`
	MaxVersion   string   `json:"max_version"`
	CipherSuites []string `json:"cipher_suites"`
	ClientAuth   string   `json:"client_auth"`
}

func runTLSSettings(cmd *cobra.Command, args []string) error {
	minVersion, _ := cmd.Flags().GetString("min-version")
	ciphers, _ := cmd.Flags().GetStringSlice("ciphers")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// If flags provided, update settings
	if minVersion != "" || len(ciphers) > 0 {
		data := make(map[string]interface{})
		if minVersion != "" {
			data["min_version"] = minVersion
		}
		if len(ciphers) > 0 {
			data["cipher_suites"] = ciphers
		}

		body, _ := json.Marshal(data)
		resp, err := client.put("/api/v1/tls/settings", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return handleAPIError(resp)
		}

		fmt.Println("TLS settings updated successfully.")
		return nil
	}

	// Otherwise, show current settings
	resp, err := client.get("/api/v1/tls/settings")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var settings tlsSettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Minimum TLS Version: %s\n", settings.MinVersion)
	fmt.Printf("Maximum TLS Version: %s\n", settings.MaxVersion)
	fmt.Printf("Client Auth:         %s\n", settings.ClientAuth)
	fmt.Println("Cipher Suites:")
	for _, cipher := range settings.CipherSuites {
		fmt.Printf("  - %s\n", cipher)
	}

	return nil
}
