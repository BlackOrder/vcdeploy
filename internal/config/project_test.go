package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantErr   bool
		checkFunc func(*testing.T, *ProjectConfig)
	}{
		{
			name: "valid agent-based config",
			content: `
name: test-project
repository: https://github.com/test/repo.git
type: nodejs
deployment:
  on_busy: queue
  strategy: symlink
  keep_releases: 10
targets:
  production:
    agent: prod-agent
    branch: main
    path: /var/www/app
hooks:
  pre_deploy:
    - npm ci
  post_deploy:
    - pm2 restart app
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *ProjectConfig) {
				if c.Name != "test-project" {
					t.Errorf("unexpected name: %s", c.Name)
				}
				if c.Repository != "https://github.com/test/repo.git" {
					t.Errorf("unexpected repository: %s", c.Repository)
				}
				if c.Deployment.OnBusy != "queue" {
					t.Errorf("unexpected on_busy: %s", c.Deployment.OnBusy)
				}
				if c.Deployment.KeepReleases != 10 {
					t.Errorf("unexpected keep_releases: %d", c.Deployment.KeepReleases)
				}
				if len(c.Targets) != 1 {
					t.Errorf("unexpected targets count: %d", len(c.Targets))
				}
				target, ok := c.Targets["production"]
				if !ok {
					t.Error("production target not found")
				}
				if target.Agent != "prod-agent" {
					t.Errorf("unexpected agent: %s", target.Agent)
				}
			},
		},
		{
			name: "valid SSH-based config",
			content: `
name: ssh-project
repository: git@github.com:test/repo.git
targets:
  staging:
    ssh:
      host: staging.example.com
      user: deploy
      key: ~/.ssh/id_ed25519
    branch: develop
    path: /var/www/staging
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *ProjectConfig) {
				target := c.Targets["staging"]
				if target.SSH == nil {
					t.Fatal("expected SSH config")
				}
				if target.SSH.Host != "staging.example.com" {
					t.Errorf("unexpected SSH host: %s", target.SSH.Host)
				}
				if target.SSH.User != "deploy" {
					t.Errorf("unexpected SSH user: %s", target.SSH.User)
				}
				if !target.IsSSH() {
					t.Error("IsSSH should return true")
				}
			},
		},
		{
			name: "multiple agents config",
			content: `
name: multi-agent-project
repository: https://github.com/test/repo.git
targets:
  production:
    agents:
      - agent-1
      - agent-2
      - agent-3
    branch: main
    path: /var/www/app
    deploy_strategy: rolling
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *ProjectConfig) {
				target := c.Targets["production"]
				agents := target.GetAgents()
				if len(agents) != 3 {
					t.Errorf("expected 3 agents, got %d", len(agents))
				}
				if target.DeployStrategy != "rolling" {
					t.Errorf("unexpected deploy_strategy: %s", target.DeployStrategy)
				}
			},
		},
		{
			name: "defaults applied",
			content: `
name: defaults-project
repository: https://github.com/test/repo.git
targets:
  prod:
    agent: agent-1
    branch: main
    path: /app
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *ProjectConfig) {
				if c.Deployment.OnBusy != "cancel" {
					t.Errorf("expected default on_busy=cancel, got: %s", c.Deployment.OnBusy)
				}
				if c.Deployment.Strategy != "symlink" {
					t.Errorf("expected default strategy=symlink, got: %s", c.Deployment.Strategy)
				}
				if c.Deployment.KeepReleases != 5 {
					t.Errorf("expected default keep_releases=5, got: %d", c.Deployment.KeepReleases)
				}
				if c.Env.PlaceholderPattern != "${SECRET_NAME}" {
					t.Errorf("expected default placeholder pattern, got: %s", c.Env.PlaceholderPattern)
				}
			},
		},
		{
			name: "hooks and notifications",
			content: `
name: hooks-project
repository: https://github.com/test/repo.git
targets:
  prod:
    agent: agent-1
    branch: main
    path: /app
hooks:
  pre_deploy:
    - composer install
    - php artisan migrate
  post_deploy:
    - php artisan config:cache
  reload:
    - service: php-fpm
      action: reload
    - service: nginx
      action: reload
  rollback:
    - php artisan migrate:rollback
notifications:
  on_success:
    - slack: "#deployments"
  on_failure:
    - slack: "#alerts"
    - email: ops@example.com
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *ProjectConfig) {
				if len(c.Hooks.PreDeploy) != 2 {
					t.Errorf("expected 2 pre_deploy hooks, got %d", len(c.Hooks.PreDeploy))
				}
				if len(c.Hooks.Reload) != 2 {
					t.Errorf("expected 2 reload hooks, got %d", len(c.Hooks.Reload))
				}
				if c.Hooks.Reload[0].Service != "php-fpm" {
					t.Errorf("unexpected first reload service: %s", c.Hooks.Reload[0].Service)
				}
				if len(c.Notifications.OnSuccess) != 1 {
					t.Errorf("expected 1 success notification, got %d", len(c.Notifications.OnSuccess))
				}
				if len(c.Notifications.OnFailure) != 2 {
					t.Errorf("expected 2 failure notifications, got %d", len(c.Notifications.OnFailure))
				}
			},
		},
		{
			name: "health check config",
			content: `
name: health-project
repository: https://github.com/test/repo.git
targets:
  prod:
    agent: agent-1
    branch: main
    path: /app
health:
  url: http://localhost:8080/health
  timeout: 30s
  retries: 3
  rollback_on_fail: true
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *ProjectConfig) {
				if c.Health.URL != "http://localhost:8080/health" {
					t.Errorf("unexpected health URL: %s", c.Health.URL)
				}
				if c.Health.Retries != 3 {
					t.Errorf("unexpected health retries: %d", c.Health.Retries)
				}
				if !c.Health.RollbackOnFail {
					t.Error("expected rollback_on_fail to be true")
				}
			},
		},
		{
			name:    "invalid yaml",
			content: `invalid: [yaml: broken`,
			wantErr: true,
		},
		{
			name: "watch config",
			content: `
name: watch-project
repository: https://github.com/test/repo.git
watch:
  branches:
    - main
    - develop
  actions:
    - push
    - pull_request
  guards:
    reject_force_push: true
    require_ci_pass: true
targets:
  prod:
    agent: agent-1
    branch: main
    path: /app
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *ProjectConfig) {
				if len(c.Watch.Branches) != 2 {
					t.Errorf("expected 2 watch branches, got %d", len(c.Watch.Branches))
				}
				if !c.Watch.Guards.RequireCIPass {
					t.Error("expected require_ci_pass to be true")
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "project.yaml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			config, err := LoadProjectConfig(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadProjectConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, config)
			}
		})
	}
}

func TestLoadProjectConfig_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadProjectConfig("/nonexistent/path/project.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestSaveProjectConfig(t *testing.T) {
	t.Parallel()

	config := &ProjectConfig{
		Name:       "save-test",
		Repository: "https://github.com/test/repo.git",
		Type:       "php",
		Deployment: DeploymentConfig{
			OnBusy:       "queue",
			Strategy:     "symlink",
			KeepReleases: 5,
		},
		Targets: map[string]TargetConfig{
			"production": {
				Agent:  "prod-agent",
				Branch: "main",
				Path:   "/var/www/app",
			},
		},
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "subdir", "project.yaml")

	// Save config
	if err := SaveProjectConfig(config, configPath); err != nil {
		t.Fatalf("SaveProjectConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Load and verify
	loaded, err := LoadProjectConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loaded.Name != config.Name {
		t.Errorf("name mismatch: got %s, want %s", loaded.Name, config.Name)
	}
	if loaded.Repository != config.Repository {
		t.Errorf("repository mismatch: got %s, want %s", loaded.Repository, config.Repository)
	}
}

func TestProjectConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *ProjectConfig
		wantErr bool
	}{
		{
			name: "valid agent config",
			config: &ProjectConfig{
				Name:       "valid-project",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						Agent:  "agent-1",
						Branch: "main",
						Path:   "/app",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid SSH config",
			config: &ProjectConfig{
				Name:       "valid-ssh-project",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						SSH: &SSHTargetConfig{
							Host: "server.example.com",
							User: "deploy",
						},
						Branch: "main",
						Path:   "/app",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: &ProjectConfig{
				Name:       "",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						Agent:  "agent-1",
						Branch: "main",
						Path:   "/app",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing repository",
			config: &ProjectConfig{
				Name:       "test",
				Repository: "",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						Agent:  "agent-1",
						Branch: "main",
						Path:   "/app",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no targets",
			config: &ProjectConfig{
				Name:       "test",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{},
			},
			wantErr: true,
		},
		{
			name: "invalid on_busy",
			config: &ProjectConfig{
				Name:       "test",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "invalid",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						Agent:  "agent-1",
						Branch: "main",
						Path:   "/app",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid strategy",
			config: &ProjectConfig{
				Name:       "test",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "invalid",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						Agent:  "agent-1",
						Branch: "main",
						Path:   "/app",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "target missing agent and ssh",
			config: &ProjectConfig{
				Name:       "test",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						Branch: "main",
						Path:   "/app",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "target has both agent and ssh",
			config: &ProjectConfig{
				Name:       "test",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						Agent: "agent-1",
						SSH: &SSHTargetConfig{
							Host: "server.example.com",
						},
						Branch: "main",
						Path:   "/app",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "target missing branch",
			config: &ProjectConfig{
				Name:       "test",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						Agent:  "agent-1",
						Branch: "",
						Path:   "/app",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "target missing path",
			config: &ProjectConfig{
				Name:       "test",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						Agent:  "agent-1",
						Branch: "main",
						Path:   "",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "ssh target missing host",
			config: &ProjectConfig{
				Name:       "test",
				Repository: "https://github.com/test/repo.git",
				Deployment: DeploymentConfig{
					OnBusy:   "cancel",
					Strategy: "symlink",
				},
				Targets: map[string]TargetConfig{
					"prod": {
						SSH: &SSHTargetConfig{
							Host: "",
							User: "deploy",
						},
						Branch: "main",
						Path:   "/app",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTargetConfigGetAgents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target TargetConfig
		want   []string
	}{
		{
			name: "single agent",
			target: TargetConfig{
				Agent: "agent-1",
			},
			want: []string{"agent-1"},
		},
		{
			name: "multiple agents",
			target: TargetConfig{
				Agents: []string{"agent-1", "agent-2", "agent-3"},
			},
			want: []string{"agent-1", "agent-2", "agent-3"},
		},
		{
			name: "no agents",
			target: TargetConfig{
				SSH: &SSHTargetConfig{Host: "server.example.com"},
			},
			want: nil,
		},
		{
			name: "agent takes precedence over agents",
			target: TargetConfig{
				Agent:  "single-agent",
				Agents: []string{"agent-1", "agent-2"},
			},
			want: []string{"single-agent"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.target.GetAgents()
			if len(got) != len(tt.want) {
				t.Errorf("GetAgents() returned %d agents, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("GetAgents()[%d] = %s, want %s", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTargetConfigIsSSH(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target TargetConfig
		want   bool
	}{
		{
			name: "ssh target",
			target: TargetConfig{
				SSH: &SSHTargetConfig{Host: "server.example.com"},
			},
			want: true,
		},
		{
			name: "agent target",
			target: TargetConfig{
				Agent: "agent-1",
			},
			want: false,
		},
		{
			name:   "no config",
			target: TargetConfig{},
			want:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.target.IsSSH()
			if got != tt.want {
				t.Errorf("IsSSH() = %v, want %v", got, tt.want)
			}
		})
	}
}
