// Package security provides SSH host key verification with database-backed storage.
package security

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ErrHostKeyUnknown is returned when connecting to a host with no stored key.
var ErrHostKeyUnknown = errors.New("host key is unknown: host has not been verified")

// ErrHostKeyMismatch is returned when a host's key doesn't match the stored key.
var ErrHostKeyMismatch = errors.New("host key mismatch: possible MITM attack or key rotation")

// ErrHostKeyUntrusted is returned when a host key exists but hasn't been trusted.
var ErrHostKeyUntrusted = errors.New("host key exists but has not been trusted")

// HostKeyInfo contains information about an SSH host key.
type HostKeyInfo struct {
	Hostname    string
	Port        int
	KeyType     string
	PublicKey   []byte
	Fingerprint string
}

// HostKeyStore defines the interface for SSH host key storage.
type HostKeyStore interface {
	// GetHostKey retrieves a stored host key for the given host, port, and key type.
	// Returns nil if no key is stored.
	GetHostKey(ctx context.Context, hostname string, port int, keyType string) (*StoredHostKey, error)

	// GetHostKeys retrieves all stored host keys for a given host and port.
	GetHostKeys(ctx context.Context, hostname string, port int) ([]*StoredHostKey, error)

	// ListAllKeys retrieves all stored host keys across all hosts.
	// Used for exporting to known_hosts file format.
	ListAllKeys(ctx context.Context) ([]*StoredHostKey, error)

	// StoreHostKey stores a new host key (untrusted by default).
	StoreHostKey(ctx context.Context, key *StoredHostKey) error

	// TrustHostKey marks a host key as trusted.
	TrustHostKey(ctx context.Context, hostname string, port int, keyType string, trustedBy string) error

	// DeleteHostKey removes a host key.
	DeleteHostKey(ctx context.Context, hostname string, port int, keyType string) error
}

// StoredHostKey represents a host key stored in the database.
type StoredHostKey struct {
	Hostname    string
	Port        int
	KeyType     string
	PublicKey   []byte // Raw public key bytes
	Fingerprint string // SHA256 fingerprint
	Trusted     bool
	AddedBy     string
}

// HostKeyVerifier provides SSH host key verification using a database backend.
type HostKeyVerifier struct {
	store HostKeyStore
	mu    sync.RWMutex

	// StrictMode rejects all unknown hosts (even if they could be added to the store).
	StrictMode bool

	// RequireTrust requires keys to be explicitly trusted before allowing connection.
	RequireTrust bool

	// OnUnknownHost is called when an unknown host is encountered.
	// If nil and not in StrictMode, returns ErrHostKeyUnknown.
	// If the callback returns nil, the key will be stored (untrusted).
	OnUnknownHost func(info *HostKeyInfo) error

	// OnKeyMismatch is called when a host key doesn't match the stored key.
	// If nil, returns ErrHostKeyMismatch.
	OnKeyMismatch func(stored *StoredHostKey, received *HostKeyInfo) error
}

// NewHostKeyVerifier creates a new host key verifier.
func NewHostKeyVerifier(store HostKeyStore) *HostKeyVerifier {
	return &HostKeyVerifier{
		store:        store,
		StrictMode:   false,
		RequireTrust: true,
	}
}

// HostKeyCallback returns an ssh.HostKeyCallback that uses the database for verification.
func (v *HostKeyVerifier) HostKeyCallback(ctx context.Context) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return v.VerifyHostKey(ctx, hostname, remote, key)
	}
}

// VerifyHostKey verifies a host key against the stored keys.
func (v *HostKeyVerifier) VerifyHostKey(ctx context.Context, hostname string, remote net.Addr, key ssh.PublicKey) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Parse hostname and port from the remote address
	host, portStr, err := net.SplitHostPort(hostname)
	if err != nil {
		// hostname might not include port, use it as-is
		host = hostname
		portStr = "22"
	}
	port, _ := strconv.Atoi(portStr)
	if port == 0 {
		port = 22
	}

	keyType := key.Type()
	fingerprint := FingerprintSHA256(key)

	// Check if we have a stored key for this host
	stored, err := v.store.GetHostKey(ctx, host, port, keyType)
	if err != nil && !errors.Is(err, ErrHostKeyUnknown) {
		return fmt.Errorf("checking host key: %w", err)
	}

	if stored == nil {
		// Unknown host
		return v.handleUnknownHost(ctx, host, port, key, fingerprint)
	}

	// Compare the keys
	if !keysEqual(stored.PublicKey, key.Marshal()) {
		// Key mismatch
		return v.handleKeyMismatch(stored, &HostKeyInfo{
			Hostname:    host,
			Port:        port,
			KeyType:     keyType,
			PublicKey:   key.Marshal(),
			Fingerprint: fingerprint,
		})
	}

	// Key matches - check if trust is required
	if v.RequireTrust && !stored.Trusted {
		return fmt.Errorf("%w: host %s:%d key type %s", ErrHostKeyUntrusted, host, port, keyType)
	}

	return nil
}

func (v *HostKeyVerifier) handleUnknownHost(ctx context.Context, hostname string, port int, key ssh.PublicKey, fingerprint string) error {
	if v.StrictMode {
		return fmt.Errorf("%w: %s:%d (fingerprint: %s)", ErrHostKeyUnknown, hostname, port, fingerprint)
	}

	info := &HostKeyInfo{
		Hostname:    hostname,
		Port:        port,
		KeyType:     key.Type(),
		PublicKey:   key.Marshal(),
		Fingerprint: fingerprint,
	}

	// Call the callback if provided
	if v.OnUnknownHost != nil {
		if err := v.OnUnknownHost(info); err != nil {
			return err
		}
	}

	// Store the key (untrusted)
	stored := &StoredHostKey{
		Hostname:    hostname,
		Port:        port,
		KeyType:     key.Type(),
		PublicKey:   key.Marshal(),
		Fingerprint: fingerprint,
		Trusted:     false,
		AddedBy:     "auto",
	}

	if err := v.store.StoreHostKey(ctx, stored); err != nil {
		return fmt.Errorf("storing host key: %w", err)
	}

	// If trust is required, still return error
	if v.RequireTrust {
		return fmt.Errorf("%w: %s:%d (fingerprint: %s) - key has been stored, please trust it via UI",
			ErrHostKeyUntrusted, hostname, port, fingerprint)
	}

	return nil
}

func (v *HostKeyVerifier) handleKeyMismatch(stored *StoredHostKey, received *HostKeyInfo) error {
	if v.OnKeyMismatch != nil {
		return v.OnKeyMismatch(stored, received)
	}

	return fmt.Errorf("%w: %s:%d key type %s\nStored fingerprint: %s\nReceived fingerprint: %s",
		ErrHostKeyMismatch,
		received.Hostname, received.Port, received.KeyType,
		stored.Fingerprint, received.Fingerprint)
}

// FingerprintSHA256 returns the SHA256 fingerprint of a public key.
func FingerprintSHA256(key ssh.PublicKey) string {
	hash := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.StdEncoding.EncodeToString(hash[:])
}

// keysEqual compares two public key byte slices.
func keysEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ExportToKnownHostsFile exports trusted host keys to a known_hosts file format.
// This can be used to provide a file-based known_hosts for SSH operations.
func (v *HostKeyVerifier) ExportToKnownHostsFile(ctx context.Context, filePath string, trustedOnly bool) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Get all keys from the store
	allKeys, err := v.store.ListAllKeys(ctx)
	if err != nil {
		return fmt.Errorf("listing host keys: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 - filePath is admin-controlled export destination
	if err != nil {
		return fmt.Errorf("creating known_hosts file: %w", err)
	}
	defer f.Close()

	// Write header comment
	if _, err := f.WriteString("# Auto-generated by vcdeploy - DO NOT EDIT\n"); err != nil {
		return err
	}
	if _, err := f.WriteString("# SSH host keys exported from database\n\n"); err != nil {
		return err
	}

	// Export each key
	for _, key := range allKeys {
		// Skip untrusted keys if trustedOnly is set
		if trustedOnly && !key.Trusted {
			continue
		}

		// Format hostname with port if not default
		hostPort := key.Hostname
		if key.Port != 22 {
			hostPort = fmt.Sprintf("[%s]:%d", key.Hostname, key.Port)
		}

		// Parse the public key to generate known_hosts line
		pubKey, err := ssh.ParsePublicKey(key.PublicKey)
		if err != nil {
			continue // Skip invalid keys
		}

		line := knownhosts.Line([]string{hostPort}, pubKey)
		if _, err := f.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("writing key for %s: %w", hostPort, err)
		}
	}

	return nil
}

// KnownHostsExporter provides utilities for exporting host keys to known_hosts format.
type KnownHostsExporter struct {
	store HostKeyStore
}

// NewKnownHostsExporter creates a new exporter.
func NewKnownHostsExporter(store HostKeyStore) *KnownHostsExporter {
	return &KnownHostsExporter{store: store}
}

// ExportHost exports a single host's keys to known_hosts line format.
func (e *KnownHostsExporter) ExportHost(ctx context.Context, hostname string, port int) ([]string, error) {
	keys, err := e.store.GetHostKeys(ctx, hostname, port)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, key := range keys {
		if !key.Trusted {
			continue
		}

		// Format: hostname,port keytype base64-key
		hostPort := hostname
		if port != 22 {
			hostPort = fmt.Sprintf("[%s]:%d", hostname, port)
		}

		pubKey, err := ssh.ParsePublicKey(key.PublicKey)
		if err != nil {
			continue // Skip invalid keys
		}

		line := knownhosts.Line([]string{hostPort}, pubKey)
		lines = append(lines, line)
	}

	return lines, nil
}

// ExportAll exports all trusted host keys to a known_hosts file.
func (e *KnownHostsExporter) ExportAll(ctx context.Context, filePath string) error {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 - filePath is admin-controlled export destination
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	// Write header comment
	if _, err := f.WriteString("# Auto-generated by vcdeploy - DO NOT EDIT\n"); err != nil {
		return err
	}
	if _, err := f.WriteString("# Trusted SSH host keys exported from database\n\n"); err != nil {
		return err
	}

	// Get all keys from the store
	allKeys, err := e.store.ListAllKeys(ctx)
	if err != nil {
		return fmt.Errorf("listing host keys: %w", err)
	}

	// Export each trusted key
	for _, key := range allKeys {
		if !key.Trusted {
			continue
		}

		// Format hostname with port if not default
		hostPort := key.Hostname
		if key.Port != 22 {
			hostPort = fmt.Sprintf("[%s]:%d", key.Hostname, key.Port)
		}

		// Parse the public key to generate known_hosts line
		pubKey, err := ssh.ParsePublicKey(key.PublicKey)
		if err != nil {
			continue // Skip invalid keys
		}

		line := knownhosts.Line([]string{hostPort}, pubKey)
		if _, err := f.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("writing key for %s: %w", hostPort, err)
		}
	}

	return nil
}

// ParseKnownHostsLine parses a single known_hosts line and returns host key info.
func ParseKnownHostsLine(line string) (*HostKeyInfo, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil, nil // Comment or empty line
	}

	parts := strings.Fields(line)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid known_hosts line format")
	}

	// Parse hostname (may include port like [hostname]:port)
	hostname := parts[0]
	port := 22
	if strings.HasPrefix(hostname, "[") {
		// Format: [hostname]:port
		idx := strings.LastIndex(hostname, "]:")
		if idx > 0 {
			portStr := hostname[idx+2:]
			hostname = hostname[1:idx]
			if p, err := strconv.Atoi(portStr); err == nil {
				port = p
			}
		}
	}

	keyType := parts[1]
	keyData := parts[2]

	pubKeyBytes, err := base64.StdEncoding.DecodeString(keyData)
	if err != nil {
		return nil, fmt.Errorf("decoding key: %w", err)
	}

	pubKey, err := ssh.ParsePublicKey(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing key: %w", err)
	}

	return &HostKeyInfo{
		Hostname:    hostname,
		Port:        port,
		KeyType:     keyType,
		PublicKey:   pubKey.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey),
	}, nil
}
