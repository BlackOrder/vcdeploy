// Package server provides the master daemon HTTP and gRPC servers.
package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ReauthOnlyServer handles only the Reauthenticate RPC on the dedicated re-auth port.
// This server does NOT require client certificates, allowing agents with expired
// certificates to re-authenticate using their HMAC secret.
type ReauthOnlyServer struct {
	proto.UnimplementedAgentServiceServer
	store       storage.Store
	caManager   *security.CAManager
	certAuditor *security.CertAuditor
	logger      *zap.Logger
}

// NewReauthOnlyServer creates a new re-authentication only server.
func NewReauthOnlyServer(store storage.Store, caManager *security.CAManager, certAuditor *security.CertAuditor, logger *zap.Logger) *ReauthOnlyServer {
	return &ReauthOnlyServer{
		store:       store,
		caManager:   caManager,
		certAuditor: certAuditor,
		logger:      logger.Named("reauth-grpc"),
	}
}

// Reauthenticate allows an agent to obtain a new certificate using HMAC authentication.
// This RPC is available on the dedicated re-auth port which does not require mTLS.
func (s *ReauthOnlyServer) Reauthenticate(ctx context.Context, req *proto.ReauthRequest) (*proto.ReauthResponse, error) {
	// Validate required fields
	if req.AgentId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id is required")
	}
	if len(req.Nonce) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "nonce is required")
	}
	if len(req.Signature) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "signature is required")
	}

	// Get agent from database
	agent, err := s.store.GetAgent(ctx, req.AgentId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.logger.Warn("Reauthenticate: agent not found", zap.String("agent_id", req.AgentId))
			return nil, status.Errorf(codes.NotFound, "agent not found")
		}
		s.logger.Error("Reauthenticate: failed to get agent", zap.String("agent_id", req.AgentId), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to lookup agent")
	}

	// Verify agent has an HMAC secret
	if len(agent.HMACSecret) == 0 {
		s.logger.Warn("Reauthenticate: agent has no HMAC secret", zap.String("agent_id", req.AgentId))
		return nil, status.Errorf(codes.FailedPrecondition, "agent has no HMAC secret")
	}

	// Check timestamp is within acceptable window (±5 minutes)
	now := time.Now().Unix()
	const timestampTolerance = 300 // 5 minutes
	if req.Timestamp < now-timestampTolerance || req.Timestamp > now+timestampTolerance {
		s.logger.Warn("Reauthenticate: timestamp out of range",
			zap.String("agent_id", req.AgentId),
			zap.Int64("request_ts", req.Timestamp),
			zap.Int64("server_ts", now))
		return nil, status.Errorf(codes.Unauthenticated, "timestamp out of range")
	}

	// Verify HMAC signature: HMAC-SHA256(agent_id + ":" + timestamp + ":" + nonce)
	message := fmt.Sprintf("%s:%d:", req.AgentId, req.Timestamp)
	message += string(req.Nonce)

	mac := hmac.New(sha256.New, agent.HMACSecret)
	mac.Write([]byte(message))
	expected := mac.Sum(nil)

	if !hmac.Equal(expected, req.Signature) {
		s.logger.Warn("Reauthenticate: invalid HMAC signature", zap.String("agent_id", req.AgentId))
		return nil, status.Errorf(codes.Unauthenticated, "invalid signature")
	}

	// Issue new certificate
	cert, err := s.caManager.IssueAgentCertificate(ctx, req.AgentId, agent.Hostname)
	if err != nil {
		s.logger.Error("Reauthenticate: failed to issue certificate",
			zap.String("agent_id", req.AgentId),
			zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to issue certificate")
	}

	// Log audit event
	if s.certAuditor != nil {
		clientIP := ExtractGRPCClientIP(ctx)
		_ = s.certAuditor.LogRenewed(ctx, req.AgentId, cert.SerialNumber, cert.CAID, "hmac_reauth", clientIP)
	}

	s.logger.Info("Agent reauthenticated successfully",
		zap.String("agent_id", req.AgentId),
		zap.String("serial", cert.SerialNumber))

	// Get current CA PEM
	var caCertPEM []byte
	if currentCA := s.caManager.GetCurrentCA(); currentCA != nil {
		caCertPEM = []byte(currentCA.CertificatePEM)
	}

	return &proto.ReauthResponse{
		Certificate:   []byte(cert.CertificatePEM + cert.PrivateKeyPEM),
		CaCertificate: caCertPEM,
	}, nil
}

// reauthOnlyInterceptor rejects all RPCs except Reauthenticate.
// This ensures only re-authentication is possible on the dedicated port.
func reauthOnlyInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if info.FullMethod != "/vcdeploy.v1.AgentService/Reauthenticate" {
		return nil, status.Errorf(codes.Unimplemented, "only Reauthenticate is allowed on this port")
	}
	return handler(ctx, req)
}
