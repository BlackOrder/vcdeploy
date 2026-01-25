// Package testutil provides shared testing utilities for vcdeploy.
package testutil

import (
	"context"
	"io"
	"sync"
	"time"
)

// User represents a user for typed mocks.
type User struct {
	ID                 int64
	Username           string
	PasswordHash       string
	Email              string
	Role               string
	TOTPSecret         string
	TOTPEnabled        bool
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Agent represents an agent for typed mocks.
type Agent struct {
	ID           string
	Hostname     string
	Labels       map[string]string
	Capabilities string
	Status       string
	LastSeenAt   time.Time
	RegisteredAt time.Time
	Certificate  string
}

// Project represents a project for typed mocks.
type Project struct {
	ID              int64
	Name            string
	Repository      string
	Branch          string
	DeployPath      string
	Type            string
	WebhookSecret   string
	AutoDeploy      bool
	AllowedBranches string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Deployment represents a deployment for typed mocks.
type Deployment struct {
	ID          string
	ProjectID   int64
	ProjectName string
	AgentID     string
	Version     string
	Branch      string
	Commit      string
	Status      string
	StartedAt   time.Time
	FinishedAt  *time.Time
	StartedBy   string
	Error       string
}

// MockDB implements a mock database for testing with typed functions.
type MockDB struct {
	mu sync.Mutex

	// Track calls
	Calls []string

	// Typed mock returns for User operations
	GetUserFunc    func(id int64) (*User, error)
	GetUsersFunc   func() ([]*User, error)
	CreateUserFunc func(user *User) error
	UpdateUserFunc func(user *User) error
	DeleteUserFunc func(id int64) error

	// Typed mock returns for Agent operations
	GetAgentFunc    func(id string) (*Agent, error)
	GetAgentsFunc   func() ([]*Agent, error)
	CreateAgentFunc func(agent *Agent) error
	UpdateAgentFunc func(agent *Agent) error
	DeleteAgentFunc func(id string) error

	// Typed mock returns for Project operations
	GetProjectFunc    func(id int64) (*Project, error)
	GetProjectsFunc   func() ([]*Project, error)
	CreateProjectFunc func(project *Project) error
	UpdateProjectFunc func(project *Project) error
	DeleteProjectFunc func(id int64) error

	// Typed mock returns for Deployment operations
	GetDeploymentFunc    func(id string) (*Deployment, error)
	GetDeploymentsFunc   func() ([]*Deployment, error)
	CreateDeploymentFunc func(deployment *Deployment) error
	UpdateDeploymentFunc func(deployment *Deployment) error

	// Generic mock data store (for backward compatibility)
	Data map[string]interface{}
}

// NewMockDB creates a new mock database.
func NewMockDB() *MockDB {
	return &MockDB{
		Calls: make([]string, 0),
		Data:  make(map[string]interface{}),
	}
}

// RecordCall records a method call for verification.
func (m *MockDB) RecordCall(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, method)
}

// WasCalled checks if a method was called.
func (m *MockDB) WasCalled(method string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.Calls {
		if c == method {
			return true
		}
	}
	return false
}

// CallCount returns how many times a method was called.
func (m *MockDB) CallCount(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, c := range m.Calls {
		if c == method {
			count++
		}
	}
	return count
}

// MockSSHClient implements a mock SSH client for testing.
type MockSSHClient struct {
	mu sync.Mutex

	// Track calls
	ConnectCalled bool
	RunCalls      []string
	CloseCalled   bool

	// Mock returns
	ConnectError error
	RunOutput    map[string]string
	RunError     map[string]error
}

// NewMockSSHClient creates a new mock SSH client.
func NewMockSSHClient() *MockSSHClient {
	return &MockSSHClient{
		RunCalls:  make([]string, 0),
		RunOutput: make(map[string]string),
		RunError:  make(map[string]error),
	}
}

// Connect simulates connecting to SSH server.
func (m *MockSSHClient) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnectCalled = true
	return m.ConnectError
}

// Run simulates running a command.
func (m *MockSSHClient) Run(cmd string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunCalls = append(m.RunCalls, cmd)
	if err, ok := m.RunError[cmd]; ok {
		return err
	}
	return nil
}

// RunWithOutput simulates running a command and returning output.
func (m *MockSSHClient) RunWithOutput(cmd string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunCalls = append(m.RunCalls, cmd)
	output := m.RunOutput[cmd]
	err := m.RunError[cmd]
	return output, err
}

// Close simulates closing the connection.
func (m *MockSSHClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CloseCalled = true
	return nil
}

// MockGRPCStream implements a mock gRPC bidirectional stream for testing.
type MockGRPCStream struct {
	mu sync.Mutex

	// Received messages (typed for proto messages)
	ReceivedMsgs []interface{}
	// Messages to send back
	SendMsgs []interface{}
	sendIdx  int

	// Control
	SendError error
	RecvError error
	Ctx       context.Context
}

// NewMockGRPCStream creates a new mock gRPC stream.
func NewMockGRPCStream(ctx context.Context) *MockGRPCStream {
	return &MockGRPCStream{
		ReceivedMsgs: make([]interface{}, 0),
		SendMsgs:     make([]interface{}, 0),
		Ctx:          ctx,
	}
}

// Send simulates sending a message.
func (m *MockGRPCStream) Send(msg interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SendError != nil {
		return m.SendError
	}
	m.ReceivedMsgs = append(m.ReceivedMsgs, msg)
	return nil
}

// Recv simulates receiving a message.
func (m *MockGRPCStream) Recv() (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.RecvError != nil {
		return nil, m.RecvError
	}
	if m.sendIdx >= len(m.SendMsgs) {
		return nil, io.EOF
	}
	msg := m.SendMsgs[m.sendIdx]
	m.sendIdx++
	return msg, nil
}

// Context returns the stream context.
func (m *MockGRPCStream) Context() context.Context {
	return m.Ctx
}

// AddSendMsg adds a message to be returned by Recv.
func (m *MockGRPCStream) AddSendMsg(msg interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SendMsgs = append(m.SendMsgs, msg)
}

// GetReceivedMsgs returns all messages received by Send.
func (m *MockGRPCStream) GetReceivedMsgs() []interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]interface{}, len(m.ReceivedMsgs))
	copy(result, m.ReceivedMsgs)
	return result
}

// MockHTTPClient implements a mock HTTP client for testing.
type MockHTTPClient struct {
	mu sync.Mutex

	// Track requests
	Requests []MockHTTPRequest

	// Mock responses
	Responses       map[string]MockHTTPResponse
	DefaultResponse MockHTTPResponse
}

// MockHTTPRequest represents a recorded HTTP request.
type MockHTTPRequest struct {
	Method  string
	URL     string
	Body    []byte
	Headers map[string]string
}

// MockHTTPResponse represents a mock HTTP response.
type MockHTTPResponse struct {
	StatusCode int
	Body       []byte
	Error      error
}

// NewMockHTTPClient creates a new mock HTTP client.
func NewMockHTTPClient() *MockHTTPClient {
	return &MockHTTPClient{
		Requests:  make([]MockHTTPRequest, 0),
		Responses: make(map[string]MockHTTPResponse),
		DefaultResponse: MockHTTPResponse{
			StatusCode: 200,
			Body:       []byte("{}"),
		},
	}
}

// RecordRequest records an HTTP request.
func (m *MockHTTPClient) RecordRequest(method, url string, body []byte, headers map[string]string) MockHTTPResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Requests = append(m.Requests, MockHTTPRequest{
		Method:  method,
		URL:     url,
		Body:    body,
		Headers: headers,
	})
	key := method + " " + url
	if resp, ok := m.Responses[key]; ok {
		return resp
	}
	return m.DefaultResponse
}

// SetResponse sets a mock response for a specific request.
func (m *MockHTTPClient) SetResponse(method, url string, resp MockHTTPResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := method + " " + url
	m.Responses[key] = resp
}

// GetRequests returns all recorded requests.
func (m *MockHTTPClient) GetRequests() []MockHTTPRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockHTTPRequest, len(m.Requests))
	copy(result, m.Requests)
	return result
}
