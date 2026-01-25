package webhooks

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

// MockProjectStore implements ProjectStore for testing.
type MockProjectStore struct {
	Configs   map[string]*WebhookConfig
	Projects  map[string]*Project
	ConfigErr error
	GetErr    error
}

func (m *MockProjectStore) GetProjectWebhookConfig(projectID string) (*WebhookConfig, error) {
	if m.ConfigErr != nil {
		return nil, m.ConfigErr
	}
	return m.Configs[projectID], nil
}

func (m *MockProjectStore) GetProjectByID(ctx context.Context, projectID string) (*Project, error) {
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	return m.Projects[projectID], nil
}

// MockOrchestrator implements DeploymentOrchestrator for testing.
type MockOrchestrator struct {
	Deployments []MockDeployment
	Error       error
}

type MockDeployment struct {
	ProjectID     string
	Branch        string
	Commit        string
	TriggeredBy   string
	TriggerSource string
}

func (m *MockOrchestrator) TriggerDeploy(ctx context.Context, projectID, branch, commit, triggeredBy, triggerSource string) (string, error) {
	if m.Error != nil {
		return "", m.Error
	}
	m.Deployments = append(m.Deployments, MockDeployment{
		ProjectID:     projectID,
		Branch:        branch,
		Commit:        commit,
		TriggeredBy:   triggeredBy,
		TriggerSource: triggerSource,
	})
	return "test-deployment-id", nil
}

func newTestProcessor(projects ProjectStore, orchestrator DeploymentOrchestrator) *DefaultEventProcessor {
	return NewEventProcessor(EventProcessorConfig{
		Logger:        zap.NewNop(),
		Projects:      projects,
		Orchestrator:  orchestrator,
		DefaultBranch: "main",
	})
}

// TestProcessPush tests the push event processing.
func TestProcessPush(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:            true,
				AutoDeploy:         true,
				AutoDeployBranches: []string{"main", "develop"},
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &PushEvent{
		ProjectID: "project-1",
		Branch:    "main",
		Commit:    "abc123",
		Message:   "Test commit",
		Author:    "testuser",
		Provider:  "github",
		Deleted:   false,
	}

	err := processor.ProcessPush(event)
	if err != nil {
		t.Fatalf("ProcessPush() error = %v", err)
	}

	if len(orchestrator.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(orchestrator.Deployments))
	}

	d := orchestrator.Deployments[0]
	if d.ProjectID != "project-1" {
		t.Errorf("expected project-1, got %s", d.ProjectID)
	}
	if d.Branch != "main" {
		t.Errorf("expected main, got %s", d.Branch)
	}
	if d.Commit != "abc123" {
		t.Errorf("expected abc123, got %s", d.Commit)
	}
}

func TestProcessPush_DeletedBranch(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:    true,
				AutoDeploy: true,
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &PushEvent{
		ProjectID: "project-1",
		Branch:    "main",
		Deleted:   true, // Branch was deleted
		Provider:  "github",
	}

	err := processor.ProcessPush(event)
	if err != nil {
		t.Fatalf("ProcessPush() error = %v", err)
	}

	// No deployment should be triggered for deleted branches
	if len(orchestrator.Deployments) != 0 {
		t.Errorf("expected 0 deployments for deleted branch, got %d", len(orchestrator.Deployments))
	}
}

func TestProcessPush_AutoDeployDisabled(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:    true,
				AutoDeploy: false, // Auto-deploy disabled
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &PushEvent{
		ProjectID: "project-1",
		Branch:    "main",
		Commit:    "abc123",
		Provider:  "github",
	}

	err := processor.ProcessPush(event)
	if err != nil {
		t.Fatalf("ProcessPush() error = %v", err)
	}

	if len(orchestrator.Deployments) != 0 {
		t.Errorf("expected 0 deployments when auto-deploy disabled, got %d", len(orchestrator.Deployments))
	}
}

func TestProcessPush_BranchNotAllowed(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:            true,
				AutoDeploy:         true,
				AutoDeployBranches: []string{"main", "develop"}, // Only main and develop
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &PushEvent{
		ProjectID: "project-1",
		Branch:    "feature/test", // Not in allowed list
		Commit:    "abc123",
		Provider:  "github",
	}

	err := processor.ProcessPush(event)
	if err != nil {
		t.Fatalf("ProcessPush() error = %v", err)
	}

	if len(orchestrator.Deployments) != 0 {
		t.Errorf("expected 0 deployments for non-allowed branch, got %d", len(orchestrator.Deployments))
	}
}

func TestProcessPush_WildcardBranch(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:            true,
				AutoDeploy:         true,
				AutoDeployBranches: []string{"release/*"}, // Wildcard pattern
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &PushEvent{
		ProjectID: "project-1",
		Branch:    "release/v1.0.0", // Should match wildcard
		Commit:    "abc123",
		Provider:  "github",
	}

	err := processor.ProcessPush(event)
	if err != nil {
		t.Fatalf("ProcessPush() error = %v", err)
	}

	if len(orchestrator.Deployments) != 1 {
		t.Errorf("expected 1 deployment for wildcard match, got %d", len(orchestrator.Deployments))
	}
}

func TestProcessPush_ConfigError(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		ConfigErr: errors.New("database error"),
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &PushEvent{
		ProjectID: "project-1",
		Branch:    "main",
		Commit:    "abc123",
		Provider:  "github",
	}

	err := processor.ProcessPush(event)
	if err == nil {
		t.Error("expected error when config lookup fails")
	}
}

func TestProcessPush_DeployError(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:    true,
				AutoDeploy: true,
			},
		},
	}

	orchestrator := &MockOrchestrator{
		Error: errors.New("deployment failed"),
	}
	processor := newTestProcessor(projects, orchestrator)

	event := &PushEvent{
		ProjectID: "project-1",
		Branch:    "main",
		Commit:    "abc123",
		Provider:  "github",
	}

	err := processor.ProcessPush(event)
	if err == nil {
		t.Error("expected error when deployment fails")
	}
}

// TestProcessPullRequest tests the PR event processing.
func TestProcessPullRequest(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:        true,
				AutoDeployOnPR: true,
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &PullRequestEvent{
		ProjectID:    "project-1",
		Action:       "opened",
		Number:       42,
		Title:        "Test PR",
		Author:       "testuser",
		SourceBranch: "feature/test",
		TargetBranch: "main",
		Provider:     "github",
	}

	err := processor.ProcessPullRequest(event)
	if err != nil {
		t.Fatalf("ProcessPullRequest() error = %v", err)
	}

	if len(orchestrator.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(orchestrator.Deployments))
	}

	d := orchestrator.Deployments[0]
	if d.Branch != "feature/test" {
		t.Errorf("expected feature/test, got %s", d.Branch)
	}
}

func TestProcessPullRequest_Closed(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:        true,
				AutoDeployOnPR: true,
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &PullRequestEvent{
		ProjectID: "project-1",
		Action:    "closed", // PR closed, no deploy needed
		Number:    42,
		Provider:  "github",
	}

	err := processor.ProcessPullRequest(event)
	if err != nil {
		t.Fatalf("ProcessPullRequest() error = %v", err)
	}

	if len(orchestrator.Deployments) != 0 {
		t.Errorf("expected 0 deployments for closed PR, got %d", len(orchestrator.Deployments))
	}
}

func TestProcessPullRequest_Disabled(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:        true,
				AutoDeployOnPR: false, // PR deploy disabled
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &PullRequestEvent{
		ProjectID: "project-1",
		Action:    "opened",
		Number:    42,
		Provider:  "github",
	}

	err := processor.ProcessPullRequest(event)
	if err != nil {
		t.Fatalf("ProcessPullRequest() error = %v", err)
	}

	if len(orchestrator.Deployments) != 0 {
		t.Errorf("expected 0 deployments when PR deploy disabled, got %d", len(orchestrator.Deployments))
	}
}

// TestProcessTag tests the tag event processing.
func TestProcessTag(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:         true,
				AutoDeployOnTag: true,
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &TagEvent{
		ProjectID: "project-1",
		Tag:       "v1.0.0",
		Commit:    "abc123",
		Author:    "testuser",
		Provider:  "github",
		Deleted:   false,
	}

	err := processor.ProcessTag(event)
	if err != nil {
		t.Fatalf("ProcessTag() error = %v", err)
	}

	if len(orchestrator.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(orchestrator.Deployments))
	}

	d := orchestrator.Deployments[0]
	if d.Branch != "refs/tags/v1.0.0" {
		t.Errorf("expected refs/tags/v1.0.0, got %s", d.Branch)
	}
	if d.Commit != "abc123" {
		t.Errorf("expected abc123, got %s", d.Commit)
	}
}

func TestProcessTag_Deleted(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:         true,
				AutoDeployOnTag: true,
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &TagEvent{
		ProjectID: "project-1",
		Tag:       "v1.0.0",
		Deleted:   true, // Tag was deleted
		Provider:  "github",
	}

	err := processor.ProcessTag(event)
	if err != nil {
		t.Fatalf("ProcessTag() error = %v", err)
	}

	if len(orchestrator.Deployments) != 0 {
		t.Errorf("expected 0 deployments for deleted tag, got %d", len(orchestrator.Deployments))
	}
}

func TestProcessTag_Disabled(t *testing.T) {
	t.Parallel()

	projects := &MockProjectStore{
		Configs: map[string]*WebhookConfig{
			"project-1": {
				Enabled:         true,
				AutoDeployOnTag: false, // Tag deploy disabled
			},
		},
	}

	orchestrator := &MockOrchestrator{}
	processor := newTestProcessor(projects, orchestrator)

	event := &TagEvent{
		ProjectID: "project-1",
		Tag:       "v1.0.0",
		Commit:    "abc123",
		Provider:  "github",
	}

	err := processor.ProcessTag(event)
	if err != nil {
		t.Fatalf("ProcessTag() error = %v", err)
	}

	if len(orchestrator.Deployments) != 0 {
		t.Errorf("expected 0 deployments when tag deploy disabled, got %d", len(orchestrator.Deployments))
	}
}

// TestShouldDeployBranch tests the branch matching logic.
func TestShouldDeployBranch(t *testing.T) {
	t.Parallel()

	processor := newTestProcessor(&MockProjectStore{}, &MockOrchestrator{})

	tests := []struct {
		name     string
		branch   string
		allowed  []string
		expected bool
	}{
		{
			name:     "exact match",
			branch:   "main",
			allowed:  []string{"main", "develop"},
			expected: true,
		},
		{
			name:     "no match",
			branch:   "feature/test",
			allowed:  []string{"main", "develop"},
			expected: false,
		},
		{
			name:     "wildcard match",
			branch:   "release/v1.0.0",
			allowed:  []string{"release/*"},
			expected: true,
		},
		{
			name:     "wildcard no match",
			branch:   "feature/test",
			allowed:  []string{"release/*"},
			expected: false,
		},
		{
			name:     "empty allowed defaults to main",
			branch:   "main",
			allowed:  []string{},
			expected: true,
		},
		{
			name:     "empty allowed defaults to master",
			branch:   "master",
			allowed:  []string{},
			expected: true,
		},
		{
			name:     "empty allowed rejects other",
			branch:   "develop",
			allowed:  []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.shouldDeployBranch(tt.branch, tt.allowed)
			if result != tt.expected {
				t.Errorf("shouldDeployBranch(%s, %v) = %v, want %v",
					tt.branch, tt.allowed, result, tt.expected)
			}
		})
	}
}

// TestTruncateMessage tests the message truncation helper.
func TestTruncateMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short message",
			input:    "Short",
			maxLen:   50,
			expected: "Short",
		},
		{
			name:     "long message",
			input:    "This is a very long message that should be truncated",
			maxLen:   20,
			expected: "This is a very lo...",
		},
		{
			name:     "message with newline",
			input:    "First line\nSecond line",
			maxLen:   50,
			expected: "First line",
		},
		{
			name:     "exact length",
			input:    "Exact",
			maxLen:   5,
			expected: "Exact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateMessage(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateMessage(%q, %d) = %q, want %q",
					tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}
