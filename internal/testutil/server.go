package testutil

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestServer wraps httptest.Server with mTLS support.
type TestServer struct {
	*httptest.Server

	// CABundle is the CA infrastructure used for this server.
	// Only set when created with NewTestServerWithCA.
	CABundle *TestCABundle

	// TrustPool contains trusted CA certificates for client connections.
	TrustPool *x509.CertPool

	// ClientCert is a certificate clients can use to connect (for mTLS).
	ClientCert *tls.Certificate
}

// Close shuts down the server and cleans up resources.
func (s *TestServer) Close() {
	s.Server.Close()
	if s.CABundle != nil {
		s.CABundle.Close()
	}
}

// NewTLSClient returns an HTTP client configured for mTLS with this server.
func (s *TestServer) NewTLSClient() *http.Client {
	tlsConfig := &tls.Config{
		RootCAs:    s.TrustPool,
		MinVersion: tls.VersionTLS12,
	}

	if s.ClientCert != nil {
		tlsConfig.Certificates = []tls.Certificate{*s.ClientCert}
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 10 * time.Second,
	}
}

// URL returns the server URL (same as Server.URL but for convenience).
func (s *TestServer) URL() string {
	return s.Server.URL
}

// NewTestServer creates an HTTP test server without TLS.
// Use for tests that don't require TLS.
func NewTestServer(t *testing.T, handler http.Handler) *TestServer {
	t.Helper()

	server := httptest.NewServer(handler)
	return &TestServer{
		Server: server,
	}
}

// NewTestServerTLS creates an HTTPS test server with a self-signed certificate.
// The returned server has a trust pool configured to trust its certificate.
func NewTestServerTLS(t *testing.T, handler http.Handler) *TestServer {
	t.Helper()

	server := httptest.NewTLSServer(handler)

	// Get the server's certificate
	serverCert := server.TLS.Certificates[0]
	leafCert, err := x509.ParseCertificate(serverCert.Certificate[0])
	if err != nil {
		server.Close()
		t.Fatalf("testutil.NewTestServerTLS: parse server cert: %v", err)
	}

	// Create trust pool with server cert
	pool := x509.NewCertPool()
	pool.AddCert(leafCert)

	return &TestServer{
		Server:    server,
		TrustPool: pool,
	}
}

// NewTestServerWithCA creates an HTTPS test server using the CA infrastructure.
// This sets up proper mTLS with certificates issued by the test CA.
// The server requires client certificates for connections.
func NewTestServerWithCA(t *testing.T, handler http.Handler) *TestServer {
	t.Helper()

	// Create CA infrastructure
	bundle := NewTestCA(t)

	// Generate a server certificate signed by the CA
	serverCert := generateServerCert(t, bundle)

	// Issue client certificate for test connections
	agentCert := NewTestAgentCert(t, bundle, "test-client")
	clientCert := agentCert.TLSCert(t)

	// Configure TLS
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*serverCert},
		ClientCAs:    bundle.TrustPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	// Create server with custom TLS config
	server := httptest.NewUnstartedServer(handler)
	server.TLS = tlsConfig
	server.StartTLS()

	return &TestServer{
		Server:     server,
		CABundle:   bundle,
		TrustPool:  bundle.TrustPool,
		ClientCert: clientCert,
	}
}

// generateServerCert creates a server certificate signed by the test CA.
func generateServerCert(t *testing.T, bundle *TestCABundle) *tls.Certificate {
	t.Helper()

	// Get current CA
	ca := bundle.CAManager.GetCurrentCA()
	if ca == nil {
		t.Fatal("testutil.generateServerCert: no current CA")
	}

	// Generate server key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("testutil.generateServerCert: GenerateKey: %v", err)
	}

	// Create certificate template
	serialNumber, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	// Sign with CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Certificate, &priv.PublicKey, ca.PrivateKey)
	if err != nil {
		t.Fatalf("testutil.generateServerCert: CreateCertificate: %v", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}
}

// NewTestServerMTLS is an alias for NewTestServerWithCA for clarity.
// Use this when you explicitly want mTLS (mutual TLS) testing.
func NewTestServerMTLS(t *testing.T, handler http.Handler) *TestServer {
	return NewTestServerWithCA(t, handler)
}

// TestListener creates a listener for testing network code.
// Returns a listener that accepts connections on localhost.
type TestListener struct {
	net.Listener
	t *testing.T
}

// NewTestListener creates a TCP listener on a random available port.
func NewTestListener(t *testing.T) *TestListener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testutil.NewTestListener: %v", err)
	}

	t.Cleanup(func() {
		listener.Close()
	})

	return &TestListener{
		Listener: listener,
		t:        t,
	}
}

// Addr returns the listener's address in "host:port" format.
func (l *TestListener) Addr() string {
	return l.Listener.Addr().String()
}

// Port returns just the port number.
func (l *TestListener) Port() int {
	addr := l.Listener.Addr().(*net.TCPAddr)
	return addr.Port
}

// MustGet performs a GET request and fails the test if it errors.
// Returns the response body as a string.
func MustGet(t *testing.T, client *http.Client, url string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("MustGet: create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("MustGet %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("MustGet %s: read body: %v", url, err)
	}

	if resp.StatusCode >= 400 {
		t.Fatalf("MustGet %s: status %d: %s", url, resp.StatusCode, string(body))
	}

	return string(body)
}

// MustGetStatus performs a GET request and returns the status code.
// Fails if the request itself fails, but allows any status code.
func MustGetStatus(t *testing.T, client *http.Client, url string) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("MustGetStatus: create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("MustGetStatus %s: %v", url, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode
}

// AssertMTLSRequired verifies that a server requires client certificates.
// Makes a request without a client cert and expects it to fail.
func AssertMTLSRequired(t *testing.T, serverURL string, trustPool *x509.CertPool) {
	t.Helper()

	// Create client without client certificate
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    trustPool,
				MinVersion: tls.VersionTLS12,
				// Intentionally no client cert
			},
		},
		Timeout: 5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		t.Fatalf("AssertMTLSRequired: create request: %v", err)
	}

	_, err = client.Do(req)
	if err == nil {
		t.Fatal("AssertMTLSRequired: request succeeded without client cert, mTLS not enforced")
	}

	// The error should be TLS-related
	// Different Go versions and OSes report this differently,
	// so we just check that there was an error
	t.Logf("AssertMTLSRequired: correctly rejected (error: %v)", err)
}

// WaitForServer waits for a server to become available.
// Returns an error if the server doesn't respond within timeout.
func WaitForServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{
		Timeout: 1 * time.Second,
		Transport: &http.Transport{
			// #nosec G402 - InsecureSkipVerify is required for testing with self-signed certificates
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				cancel()
				return nil
			}
		}
		cancel()
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("server at %s not available after %v", url, timeout)
}
