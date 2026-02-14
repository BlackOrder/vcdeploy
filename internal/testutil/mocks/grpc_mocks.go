// Package testutil provides shared testing utilities for vcdeploy.
package testutil

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	protolib "google.golang.org/protobuf/proto"
)

// MockAgentServiceClient implements proto.AgentServiceClient for testing.
type MockAgentServiceClient struct {
	mu sync.Mutex

	// Track calls
	RegisterCalls  []*proto.RegisterRequest
	HeartbeatCalls []*proto.HeartbeatRequest
	ConnectCalled  int
	ConnectStreams []*MockAgentConnectStream

	// Mock responses
	RegisterResponse  *proto.RegisterResponse
	RegisterError     error
	HeartbeatResponse *proto.HeartbeatResponse
	HeartbeatError    error
	ConnectError      error

	// Factory for creating streams
	ConnectStreamFactory func() *MockAgentConnectStream
}

// NewMockAgentServiceClient creates a new mock agent service client.
func NewMockAgentServiceClient() *MockAgentServiceClient {
	return &MockAgentServiceClient{
		RegisterCalls:  make([]*proto.RegisterRequest, 0),
		HeartbeatCalls: make([]*proto.HeartbeatRequest, 0),
		ConnectStreams: make([]*MockAgentConnectStream, 0),
		RegisterResponse: &proto.RegisterResponse{
			Success:       true,
			Certificate:   []byte("test-certificate"),
			CaCertificate: []byte("test-ca-certificate"),
		},
		HeartbeatResponse: &proto.HeartbeatResponse{
			Ok:              true,
			ServerTimestamp: time.Now().Unix(),
		},
	}
}

// Register implements proto.AgentServiceClient.
func (m *MockAgentServiceClient) Register(ctx context.Context, in *proto.RegisterRequest, opts ...grpc.CallOption) (*proto.RegisterResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RegisterCalls = append(m.RegisterCalls, in)
	if m.RegisterError != nil {
		return nil, m.RegisterError
	}
	return m.RegisterResponse, nil
}

// Connect implements proto.AgentServiceClient.
func (m *MockAgentServiceClient) Connect(ctx context.Context, opts ...grpc.CallOption) (proto.AgentService_ConnectClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnectCalled++
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}

	var stream *MockAgentConnectStream
	if m.ConnectStreamFactory != nil {
		stream = m.ConnectStreamFactory()
	} else {
		stream = NewMockAgentConnectStream(ctx)
	}
	m.ConnectStreams = append(m.ConnectStreams, stream)
	return stream, nil
}

// Heartbeat implements proto.AgentServiceClient.
func (m *MockAgentServiceClient) Heartbeat(ctx context.Context, in *proto.HeartbeatRequest, opts ...grpc.CallOption) (*proto.HeartbeatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HeartbeatCalls = append(m.HeartbeatCalls, in)
	if m.HeartbeatError != nil {
		return nil, m.HeartbeatError
	}
	return m.HeartbeatResponse, nil
}

// GetRegisterCalls returns all register calls made.
func (m *MockAgentServiceClient) GetRegisterCalls() []*proto.RegisterRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*proto.RegisterRequest, len(m.RegisterCalls))
	copy(result, m.RegisterCalls)
	return result
}

// GetHeartbeatCalls returns all heartbeat calls made.
func (m *MockAgentServiceClient) GetHeartbeatCalls() []*proto.HeartbeatRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*proto.HeartbeatRequest, len(m.HeartbeatCalls))
	copy(result, m.HeartbeatCalls)
	return result
}

// MockAgentConnectStream implements proto.AgentService_ConnectClient for testing.
type MockAgentConnectStream struct {
	mu sync.Mutex

	ctx context.Context

	// Messages sent from agent to master (via Send)
	SentMessages []*proto.AgentMessage
	// Messages to return from master to agent (via Recv)
	RecvMessages []*proto.MasterMessage
	recvIdx      int

	// Control behavior
	SendError error
	RecvError error
	Closed    bool

	// Metadata - used for Header() and Trailer() implementations
	RecvdHeader  metadata.MD
	RecvdTrailer metadata.MD
}

// NewMockAgentConnectStream creates a new mock agent connect stream.
func NewMockAgentConnectStream(ctx context.Context) *MockAgentConnectStream {
	return &MockAgentConnectStream{
		ctx:          ctx,
		SentMessages: make([]*proto.AgentMessage, 0),
		RecvMessages: make([]*proto.MasterMessage, 0),
	}
}

// Send implements proto.AgentService_ConnectClient.
func (m *MockAgentConnectStream) Send(msg *proto.AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return io.EOF
	}
	if m.SendError != nil {
		return m.SendError
	}
	m.SentMessages = append(m.SentMessages, msg)
	return nil
}

// Recv implements proto.AgentService_ConnectClient.
func (m *MockAgentConnectStream) Recv() (*proto.MasterMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return nil, io.EOF
	}
	if m.RecvError != nil {
		return nil, m.RecvError
	}
	if m.recvIdx >= len(m.RecvMessages) {
		return nil, io.EOF
	}
	msg := m.RecvMessages[m.recvIdx]
	m.recvIdx++
	return msg, nil
}

// Header implements grpc.ClientStream.
func (m *MockAgentConnectStream) Header() (metadata.MD, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.RecvdHeader, nil
}

// Trailer implements grpc.ClientStream.
func (m *MockAgentConnectStream) Trailer() metadata.MD {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.RecvdTrailer
}

// CloseSend implements grpc.ClientStream.
func (m *MockAgentConnectStream) CloseSend() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Closed = true
	return nil
}

// Context implements grpc.ClientStream.
func (m *MockAgentConnectStream) Context() context.Context {
	return m.ctx
}

// SendMsg implements grpc.ClientStream.
func (m *MockAgentConnectStream) SendMsg(msg interface{}) error {
	if agentMsg, ok := msg.(*proto.AgentMessage); ok {
		return m.Send(agentMsg)
	}
	return nil
}

// RecvMsg implements grpc.ClientStream.
func (m *MockAgentConnectStream) RecvMsg(msg interface{}) error {
	masterMsg, err := m.Recv()
	if err != nil {
		return err
	}
	if dest, ok := msg.(*proto.MasterMessage); ok {
		// Use proto.Merge to avoid copying embedded sync.Mutex in protobuf state
		protolib.Merge(dest, masterMsg)
	}
	return nil
}

// AddRecvMessage adds a message to be returned by Recv.
func (m *MockAgentConnectStream) AddRecvMessage(msg *proto.MasterMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RecvMessages = append(m.RecvMessages, msg)
}

// GetSentMessages returns all messages sent via Send.
func (m *MockAgentConnectStream) GetSentMessages() []*proto.AgentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*proto.AgentMessage, len(m.SentMessages))
	copy(result, m.SentMessages)
	return result
}

// MockAgentServiceServer implements proto.AgentServiceServer for testing.
type MockAgentServiceServer struct {
	proto.UnimplementedAgentServiceServer
	mu sync.Mutex

	// Track calls
	RegisterCalls  []*proto.RegisterRequest
	HeartbeatCalls []*proto.HeartbeatRequest
	ConnectCalled  int

	// Mock responses
	RegisterResponse  *proto.RegisterResponse
	RegisterError     error
	HeartbeatResponse *proto.HeartbeatResponse
	HeartbeatError    error
	ConnectError      error

	// Connect handler for custom behavior
	ConnectHandler func(stream proto.AgentService_ConnectServer) error
}

// NewMockAgentServiceServer creates a new mock agent service server.
func NewMockAgentServiceServer() *MockAgentServiceServer {
	return &MockAgentServiceServer{
		RegisterCalls:  make([]*proto.RegisterRequest, 0),
		HeartbeatCalls: make([]*proto.HeartbeatRequest, 0),
		RegisterResponse: &proto.RegisterResponse{
			Success:       true,
			Certificate:   []byte("test-certificate"),
			CaCertificate: []byte("test-ca-certificate"),
		},
		HeartbeatResponse: &proto.HeartbeatResponse{
			Ok:              true,
			ServerTimestamp: time.Now().Unix(),
		},
	}
}

// Register implements proto.AgentServiceServer.
func (m *MockAgentServiceServer) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RegisterCalls = append(m.RegisterCalls, req)
	if m.RegisterError != nil {
		return nil, m.RegisterError
	}
	return m.RegisterResponse, nil
}

// Connect implements proto.AgentServiceServer.
func (m *MockAgentServiceServer) Connect(stream proto.AgentService_ConnectServer) error {
	m.mu.Lock()
	m.ConnectCalled++
	if m.ConnectError != nil {
		m.mu.Unlock()
		return m.ConnectError
	}
	if m.ConnectHandler != nil {
		handler := m.ConnectHandler
		m.mu.Unlock()
		return handler(stream)
	}
	m.mu.Unlock()

	// Default behavior: just wait for context cancellation
	<-stream.Context().Done()
	return stream.Context().Err()
}

// Heartbeat implements proto.AgentServiceServer.
func (m *MockAgentServiceServer) Heartbeat(ctx context.Context, req *proto.HeartbeatRequest) (*proto.HeartbeatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HeartbeatCalls = append(m.HeartbeatCalls, req)
	if m.HeartbeatError != nil {
		return nil, m.HeartbeatError
	}
	return m.HeartbeatResponse, nil
}

// GetRegisterCalls returns all register calls received.
func (m *MockAgentServiceServer) GetRegisterCalls() []*proto.RegisterRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*proto.RegisterRequest, len(m.RegisterCalls))
	copy(result, m.RegisterCalls)
	return result
}

// GetHeartbeatCalls returns all heartbeat calls received.
func (m *MockAgentServiceServer) GetHeartbeatCalls() []*proto.HeartbeatRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*proto.HeartbeatRequest, len(m.HeartbeatCalls))
	copy(result, m.HeartbeatCalls)
	return result
}

// MockAgentConnectServerStream implements proto.AgentService_ConnectServer for testing.
type MockAgentConnectServerStream struct {
	mu sync.Mutex

	ctx context.Context

	// Messages sent from master to agent (via Send)
	SentMessages []*proto.MasterMessage
	// Messages to return from agent to master (via Recv)
	RecvMessages []*proto.AgentMessage
	recvIdx      int

	// Control behavior
	SendError error
	RecvError error

	// Metadata
	sentHeader  metadata.MD
	sentTrailer metadata.MD
}

// NewMockAgentConnectServerStream creates a new mock server stream.
func NewMockAgentConnectServerStream(ctx context.Context) *MockAgentConnectServerStream {
	return &MockAgentConnectServerStream{
		ctx:          ctx,
		SentMessages: make([]*proto.MasterMessage, 0),
		RecvMessages: make([]*proto.AgentMessage, 0),
	}
}

// Send implements proto.AgentService_ConnectServer.
func (m *MockAgentConnectServerStream) Send(msg *proto.MasterMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SendError != nil {
		return m.SendError
	}
	m.SentMessages = append(m.SentMessages, msg)
	return nil
}

// Recv implements proto.AgentService_ConnectServer.
func (m *MockAgentConnectServerStream) Recv() (*proto.AgentMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.RecvError != nil {
		return nil, m.RecvError
	}
	if m.recvIdx >= len(m.RecvMessages) {
		return nil, io.EOF
	}
	msg := m.RecvMessages[m.recvIdx]
	m.recvIdx++
	return msg, nil
}

// SetHeader implements grpc.ServerStream.
func (m *MockAgentConnectServerStream) SetHeader(md metadata.MD) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentHeader = md
	return nil
}

// SendHeader implements grpc.ServerStream.
func (m *MockAgentConnectServerStream) SendHeader(md metadata.MD) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentHeader = md
	return nil
}

// SetTrailer implements grpc.ServerStream.
func (m *MockAgentConnectServerStream) SetTrailer(md metadata.MD) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentTrailer = md
}

// Context implements grpc.ServerStream.
func (m *MockAgentConnectServerStream) Context() context.Context {
	return m.ctx
}

// SendMsg implements grpc.ServerStream.
func (m *MockAgentConnectServerStream) SendMsg(msg interface{}) error {
	if masterMsg, ok := msg.(*proto.MasterMessage); ok {
		return m.Send(masterMsg)
	}
	return nil
}

// RecvMsg implements grpc.ServerStream.
func (m *MockAgentConnectServerStream) RecvMsg(msg interface{}) error {
	agentMsg, err := m.Recv()
	if err != nil {
		return err
	}
	if dest, ok := msg.(*proto.AgentMessage); ok {
		// Use proto.Merge to avoid copying embedded sync.Mutex in protobuf state
		protolib.Merge(dest, agentMsg)
	}
	return nil
}

// AddRecvMessage adds a message to be returned by Recv.
func (m *MockAgentConnectServerStream) AddRecvMessage(msg *proto.AgentMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RecvMessages = append(m.RecvMessages, msg)
}

// GetSentMessages returns all messages sent via Send.
func (m *MockAgentConnectServerStream) GetSentMessages() []*proto.MasterMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*proto.MasterMessage, len(m.SentMessages))
	copy(result, m.SentMessages)
	return result
}

// MockCommandRunner implements a mock command runner for agent testing.
type MockCommandRunner struct {
	mu sync.Mutex

	// Track calls
	RunCalls []MockRunCall

	// Mock responses - key is command string
	RunOutput map[string]string
	RunError  map[string]error
	RunDelay  map[string]time.Duration

	// Default behavior
	DefaultOutput string
	DefaultError  error
	DefaultDelay  time.Duration
}

// MockRunCall records a run command call.
type MockRunCall struct {
	Command  string
	Args     []string
	WorkDir  string
	Env      map[string]string
	CalledAt time.Time
	Timeout  time.Duration
}

// NewMockCommandRunner creates a new mock command runner.
func NewMockCommandRunner() *MockCommandRunner {
	return &MockCommandRunner{
		RunCalls:  make([]MockRunCall, 0),
		RunOutput: make(map[string]string),
		RunError:  make(map[string]error),
		RunDelay:  make(map[string]time.Duration),
	}
}

// Run simulates running a command.
func (m *MockCommandRunner) Run(ctx context.Context, cmd string, args []string, workDir string, env map[string]string, timeout time.Duration) (string, error) {
	m.mu.Lock()
	m.RunCalls = append(m.RunCalls, MockRunCall{
		Command:  cmd,
		Args:     args,
		WorkDir:  workDir,
		Env:      env,
		CalledAt: time.Now(),
		Timeout:  timeout,
	})

	// Get delay
	delay := m.DefaultDelay
	if d, ok := m.RunDelay[cmd]; ok {
		delay = d
	}
	m.mu.Unlock()

	// Simulate delay
	if delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for specific output/error
	if err, ok := m.RunError[cmd]; ok {
		return "", err
	}
	if output, ok := m.RunOutput[cmd]; ok {
		return output, nil
	}

	// Return defaults
	if m.DefaultError != nil {
		return "", m.DefaultError
	}
	return m.DefaultOutput, nil
}

// SetOutput sets the output for a specific command.
func (m *MockCommandRunner) SetOutput(cmd, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunOutput[cmd] = output
}

// SetError sets the error for a specific command.
func (m *MockCommandRunner) SetError(cmd string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunError[cmd] = err
}

// SetDelay sets the delay for a specific command.
func (m *MockCommandRunner) SetDelay(cmd string, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunDelay[cmd] = delay
}

// GetRunCalls returns all run calls made.
func (m *MockCommandRunner) GetRunCalls() []MockRunCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockRunCall, len(m.RunCalls))
	copy(result, m.RunCalls)
	return result
}

// ClearCalls clears all recorded calls.
func (m *MockCommandRunner) ClearCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunCalls = make([]MockRunCall, 0)
}

// MockGitClient implements a mock git client for testing deployments.
type MockGitClient struct {
	mu sync.Mutex

	// Track calls
	CloneCalls []MockGitCloneCall
	PullCalls  []MockGitPullCall
	FetchCalls []MockGitFetchCall

	// Mock responses
	CloneError error
	PullError  error
	FetchError error
	CloneDelay time.Duration
	PullDelay  time.Duration
	FetchDelay time.Duration

	// Simulated state
	CurrentCommit string
	CurrentBranch string
}

// MockGitCloneCall records a git clone call.
type MockGitCloneCall struct {
	Repo     string
	Branch   string
	DestPath string
	CalledAt time.Time
}

// MockGitPullCall records a git pull call.
type MockGitPullCall struct {
	RepoPath string
	Branch   string
	CalledAt time.Time
}

// MockGitFetchCall records a git fetch call.
type MockGitFetchCall struct {
	RepoPath string
	CalledAt time.Time
}

// NewMockGitClient creates a new mock git client.
func NewMockGitClient() *MockGitClient {
	return &MockGitClient{
		CloneCalls:    make([]MockGitCloneCall, 0),
		PullCalls:     make([]MockGitPullCall, 0),
		FetchCalls:    make([]MockGitFetchCall, 0),
		CurrentCommit: "abc1234567890",
		CurrentBranch: "main",
	}
}

// Clone simulates cloning a repository.
func (m *MockGitClient) Clone(ctx context.Context, repo, branch, destPath string) error {
	m.mu.Lock()
	m.CloneCalls = append(m.CloneCalls, MockGitCloneCall{
		Repo:     repo,
		Branch:   branch,
		DestPath: destPath,
		CalledAt: time.Now(),
	})
	delay := m.CloneDelay
	err := m.CloneError
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

// Pull simulates pulling latest changes.
func (m *MockGitClient) Pull(ctx context.Context, repoPath, branch string) error {
	m.mu.Lock()
	m.PullCalls = append(m.PullCalls, MockGitPullCall{
		RepoPath: repoPath,
		Branch:   branch,
		CalledAt: time.Now(),
	})
	delay := m.PullDelay
	err := m.PullError
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

// Fetch simulates fetching from remote.
func (m *MockGitClient) Fetch(ctx context.Context, repoPath string) error {
	m.mu.Lock()
	m.FetchCalls = append(m.FetchCalls, MockGitFetchCall{
		RepoPath: repoPath,
		CalledAt: time.Now(),
	})
	delay := m.FetchDelay
	err := m.FetchError
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

// GetCurrentCommit returns the simulated current commit hash.
func (m *MockGitClient) GetCurrentCommit() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CurrentCommit
}

// GetCurrentBranch returns the simulated current branch.
func (m *MockGitClient) GetCurrentBranch() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CurrentBranch
}

// GetCloneCalls returns all clone calls made.
func (m *MockGitClient) GetCloneCalls() []MockGitCloneCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockGitCloneCall, len(m.CloneCalls))
	copy(result, m.CloneCalls)
	return result
}

// GetPullCalls returns all pull calls made.
func (m *MockGitClient) GetPullCalls() []MockGitPullCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockGitPullCall, len(m.PullCalls))
	copy(result, m.PullCalls)
	return result
}
