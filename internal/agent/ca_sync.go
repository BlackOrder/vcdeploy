// Package agent provides the vcdeploy agent implementation.
package agent

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strconv"
	"sync"
	"time"

	pb "github.com/BlackOrder/vcdeploy/internal/proto"
	"go.uber.org/zap"
)

// CATrustManager manages CA trust pool synchronization.
type CATrustManager struct {
	store  *AgentStore
	client pb.AgentServiceClient
	logger *zap.Logger

	trustPoolMu sync.RWMutex
	trustPool   *x509.CertPool
}

// NewCATrustManager creates a new CA trust manager.
func NewCATrustManager(store *AgentStore, client pb.AgentServiceClient, logger *zap.Logger) *CATrustManager {
	return &CATrustManager{
		store:  store,
		client: client,
		logger: logger.Named("ca-trust"),
	}
}

// StartSync starts the periodic CA trust bundle synchronization.
func (m *CATrustManager) StartSync(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Initial sync after a short delay to allow connection to stabilize
	time.Sleep(5 * time.Second)
	if err := m.SyncCATrustBundle(ctx); err != nil {
		m.logger.Warn("Initial CA trust sync failed", zap.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.SyncCATrustBundle(ctx); err != nil {
				m.logger.Warn("CA trust sync failed", zap.Error(err))
			}
		}
	}
}

// SyncCATrustBundle syncs the CA trust bundle from the master.
func (m *CATrustManager) SyncCATrustBundle(ctx context.Context) error {
	if m.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// Get current version from local store
	currentVersion, _ := m.store.GetState(ctx, "ca_trust_version")

	resp, err := m.client.GetCATrustBundle(ctx, &pb.GetCATrustBundleRequest{
		CurrentVersion: string(currentVersion),
	})
	if err != nil {
		return fmt.Errorf("get CA trust bundle: %w", err)
	}

	// Check if update needed
	if resp.Version == string(currentVersion) && len(resp.CaCertificates) == 0 {
		m.logger.Debug("CA trust bundle already up to date", zap.String("version", resp.Version))
		return nil
	}

	// Store all CA certificates
	for i, caPEM := range resp.CaCertificates {
		caKey := fmt.Sprintf("ca_%d", i)
		// Parse to get expiry time (use zero time if parsing fails)
		var expiry time.Time
		block, _ := pem.Decode(caPEM)
		if block != nil {
			if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
				expiry = cert.NotAfter
			}
		}
		// SaveCertificate expects cert and optional private key - use nil for key
		if err := m.store.SaveCertificate(ctx, caKey, caPEM, nil, expiry); err != nil {
			m.logger.Warn("Failed to save CA certificate", zap.Int("index", i), zap.Error(err))
		}
	}

	// Update trust version and count
	_ = m.store.SetState(ctx, "ca_trust_version", []byte(resp.Version))
	_ = m.store.SetState(ctx, "ca_trust_count", []byte(strconv.Itoa(len(resp.CaCertificates))))
	_ = m.store.SetState(ctx, "ca_current_id", []byte(resp.CurrentCaId))

	// Rebuild local trust pool
	m.rebuildTrustPool(ctx)

	m.logger.Info("CA trust bundle updated",
		zap.String("version", resp.Version),
		zap.Int("ca_count", len(resp.CaCertificates)),
		zap.String("current_ca", resp.CurrentCaId))

	_ = m.store.LogAuditEvent(ctx, "ca_sync",
		fmt.Sprintf("synced %d CAs, version %s", len(resp.CaCertificates), resp.Version), true)

	return nil
}

// rebuildTrustPool rebuilds the local certificate trust pool from stored CA certs.
func (m *CATrustManager) rebuildTrustPool(ctx context.Context) {
	m.trustPoolMu.Lock()
	defer m.trustPoolMu.Unlock()

	pool := x509.NewCertPool()

	// Load synced CA certificates
	countStr, _ := m.store.GetState(ctx, "ca_trust_count")
	count, _ := strconv.Atoi(string(countStr))

	loadedCount := 0
	for i := 0; i < count; i++ {
		caKey := fmt.Sprintf("ca_%d", i)
		certRec, err := m.store.GetCertificate(ctx, caKey)
		if err != nil || certRec == nil {
			continue
		}
		if pool.AppendCertsFromPEM(certRec.Certificate) {
			loadedCount++
		}
	}

	// Also add the original CA cert if it exists
	if certRec, err := m.store.GetCertificate(ctx, "ca"); err == nil && certRec != nil {
		if pool.AppendCertsFromPEM(certRec.Certificate) {
			loadedCount++
		}
	}

	m.trustPool = pool
	m.logger.Debug("Trust pool rebuilt", zap.Int("loaded_cas", loadedCount))
}

// GetTrustPool returns the current trust pool for TLS verification.
func (m *CATrustManager) GetTrustPool() *x509.CertPool {
	m.trustPoolMu.RLock()
	defer m.trustPoolMu.RUnlock()
	if m.trustPool == nil {
		return x509.NewCertPool()
	}
	return m.trustPool
}
