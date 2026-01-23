// Package testutil provides shared testing utilities for vcdeploy.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// AgentConfigFixture returns a valid agent configuration YAML string.
func AgentConfigFixture() string {
	return `agent:
  id: test-agent-001
  tags:
    - test
    - unit

master:
  address: "localhost:9090"
  cert: ""
  reconnect:
    heartbeat_interval: 30s
    initial_delay: 1s
    max_delay: 5m

logging:
  level: info
  file: ""

deploy:
  work_dir: /tmp/vcdeploy-test
  timeout: 10m
  keep_releases: 5
  shared_dirs:
    - storage
  shared_files:
    - .env

git:
  depth: 1

limits:
  max_concurrent: 5
  max_log_size: 10485760

health:
  enabled: true
  port: 9091
  include_metrics: true
`
}

// MasterConfigFixture returns a valid master configuration YAML string.
func MasterConfigFixture() string {
	return `server:
  listen: ":8080"
  tls:
    enabled: false

grpc:
  listen: ":9090"

ssh:
  default_user: deploy
  default_key: ~/.ssh/id_rsa
  connection_timeout: 30s
  keepalive_interval: 10s
  idle_timeout: 5m

security:
  key_rotation:
    enabled: false
  session_timeout: 24h
  require_2fa_admin: false

backup:
  database:
    enabled: false
  config:
    versions: 5

logs:
  deployment:
    retention: 168h
    max_size_mb: 100
  audit:
    retention: 720h
    export:
      enabled: false
  application:
    level: info
    retention: 72h
  rotation:
    schedule: "0 0 * * *"

webhooks:
  github:
    enabled: true
    path: /webhook/github
  gitlab:
    enabled: true
    path: /webhook/gitlab
  bitbucket:
    enabled: true
    path: /webhook/bitbucket

notifications:
  providers:
    slack:
      enabled: false
    email:
      enabled: false
    webhook:
      enabled: false

api:
  enabled: true

appearance:
  theme: light
`
}

// ProjectConfigFixture returns a valid project configuration YAML string.
func ProjectConfigFixture() string {
	return `name: test-project
repository: https://github.com/test/project.git
branch: main
deploy_path: /var/www/test-project

type: nodejs

agents:
  - test-agent-001

hooks:
  before_deploy:
    - npm ci
  after_deploy:
    - pm2 restart app

shared_dirs:
  - node_modules
  - storage

shared_files:
  - .env

environment:
  NODE_ENV: production

notifications:
  - type: slack
    on:
      - deploy_started
      - deploy_success
      - deploy_failed
`
}

// TypeConfigFixture returns a valid type configuration YAML string.
func TypeConfigFixture() string {
	return `name: nodejs
description: Node.js application deployment

hooks:
  before_deploy:
    - npm ci --production
  after_deploy:
    - pm2 reload ecosystem.config.js

shared_dirs:
  - node_modules
  - uploads

shared_files:
  - .env

health_check:
  enabled: true
  path: /health
  timeout: 30s
  retries: 3
`
}

// InvalidYAMLFixture returns an invalid YAML string.
func InvalidYAMLFixture() string {
	return `invalid:
  - yaml: [broken
  missing: colon
`
}

// WriteFixtureFile writes a fixture to a file and returns the path.
func WriteFixtureFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}
	return path
}

// CreateTestProjectStructure creates a test project structure for deployment tests.
func CreateTestProjectStructure(t *testing.T, baseDir string) string {
	t.Helper()

	projectDir := filepath.Join(baseDir, "test-project")

	// Create directory structure
	dirs := []string{
		"releases/20240101120000",
		"releases/20240102120000",
		"shared/storage",
		"repo/.git",
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(projectDir, d), 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", d, err)
		}
	}

	// Create files
	files := map[string]string{
		"releases/20240101120000/index.html": "<html>v1</html>",
		"releases/20240102120000/index.html": "<html>v2</html>",
		"shared/.env":                        "APP_ENV=test",
		"repo/index.html":                    "<html>latest</html>",
	}

	for f, content := range files {
		path := filepath.Join(projectDir, f)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", f, err)
		}
	}

	// Create current symlink
	currentLink := filepath.Join(projectDir, "current")
	releasePath := filepath.Join(projectDir, "releases/20240102120000")
	if err := os.Symlink(releasePath, currentLink); err != nil {
		t.Fatalf("failed to create current symlink: %v", err)
	}

	return projectDir
}

// CreateTestGitRepo creates a minimal git repository for testing.
func CreateTestGitRepo(t *testing.T, dir string) string {
	t.Helper()

	repoDir := filepath.Join(dir, "repo")
	gitDir := filepath.Join(repoDir, ".git")

	// Create minimal .git structure
	dirs := []string{
		filepath.Join(gitDir, "objects"),
		filepath.Join(gitDir, "refs/heads"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("failed to create git directory: %v", err)
		}
	}

	// Create HEAD file
	headFile := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headFile, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatalf("failed to create HEAD file: %v", err)
	}

	// Create config file
	configFile := filepath.Join(gitDir, "config")
	config := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
[remote "origin"]
	url = https://github.com/test/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
`
	if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repository\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	return repoDir
}

// WebhookPayloadFixture returns sample webhook payloads for testing.
type WebhookPayloadFixture struct{}

// GitHubPush returns a GitHub push event payload.
func (WebhookPayloadFixture) GitHubPush() string {
	return `{
  "ref": "refs/heads/main",
  "before": "0000000000000000000000000000000000000000",
  "after": "abc123def456789",
  "repository": {
    "id": 12345,
    "name": "test-repo",
    "full_name": "test-org/test-repo",
    "clone_url": "https://github.com/test-org/test-repo.git",
    "ssh_url": "git@github.com:test-org/test-repo.git"
  },
  "pusher": {
    "name": "testuser",
    "email": "test@example.com"
  },
  "sender": {
    "login": "testuser",
    "id": 1
  },
  "commits": [
    {
      "id": "abc123def456789",
      "message": "Test commit",
      "author": {
        "name": "Test User",
        "email": "test@example.com"
      }
    }
  ]
}`
}

// GitHubPullRequest returns a GitHub pull request event payload.
func (WebhookPayloadFixture) GitHubPullRequest() string {
	return `{
  "action": "opened",
  "number": 1,
  "pull_request": {
    "id": 1,
    "number": 1,
    "state": "open",
    "title": "Test PR",
    "head": {
      "ref": "feature-branch",
      "sha": "abc123def456789"
    },
    "base": {
      "ref": "main",
      "sha": "def456abc789123"
    }
  },
  "repository": {
    "id": 12345,
    "name": "test-repo",
    "full_name": "test-org/test-repo"
  }
}`
}

// GitLabPush returns a GitLab push event payload.
func (WebhookPayloadFixture) GitLabPush() string {
	return `{
  "object_kind": "push",
  "event_name": "push",
  "before": "0000000000000000000000000000000000000000",
  "after": "abc123def456789",
  "ref": "refs/heads/main",
  "checkout_sha": "abc123def456789",
  "user_name": "Test User",
  "user_email": "test@example.com",
  "project": {
    "id": 12345,
    "name": "test-repo",
    "path_with_namespace": "test-org/test-repo",
    "git_http_url": "https://gitlab.com/test-org/test-repo.git",
    "git_ssh_url": "git@gitlab.com:test-org/test-repo.git"
  },
  "commits": [
    {
      "id": "abc123def456789",
      "message": "Test commit",
      "author": {
        "name": "Test User",
        "email": "test@example.com"
      }
    }
  ]
}`
}

// GitLabMergeRequest returns a GitLab merge request event payload.
func (WebhookPayloadFixture) GitLabMergeRequest() string {
	return `{
  "object_kind": "merge_request",
  "event_type": "merge_request",
  "object_attributes": {
    "id": 1,
    "iid": 1,
    "title": "Test MR",
    "state": "opened",
    "action": "open",
    "source_branch": "feature-branch",
    "target_branch": "main",
    "last_commit": {
      "id": "abc123def456789"
    }
  },
  "project": {
    "id": 12345,
    "name": "test-repo",
    "path_with_namespace": "test-org/test-repo"
  }
}`
}

// BitbucketPush returns a Bitbucket push event payload.
func (WebhookPayloadFixture) BitbucketPush() string {
	return `{
  "push": {
    "changes": [
      {
        "old": {
          "name": "main",
          "target": {
            "hash": "def456abc789123"
          }
        },
        "new": {
          "name": "main",
          "target": {
            "hash": "abc123def456789",
            "message": "Test commit",
            "author": {
              "raw": "Test User <test@example.com>"
            }
          }
        }
      }
    ]
  },
  "repository": {
    "uuid": "{12345-67890}",
    "name": "test-repo",
    "full_name": "test-org/test-repo",
    "links": {
      "html": {
        "href": "https://bitbucket.org/test-org/test-repo"
      }
    }
  },
  "actor": {
    "display_name": "Test User",
    "uuid": "{user-uuid}"
  }
}`
}

// BitbucketPullRequest returns a Bitbucket pull request event payload.
func (WebhookPayloadFixture) BitbucketPullRequest() string {
	return fmt.Sprintf(`{
  "pullrequest": {
    "id": 1,
    "title": "Test PR",
    "state": "OPEN",
    "source": {
      "branch": {
        "name": "feature-branch"
      },
      "commit": {
        "hash": "abc123def456789"
      }
    },
    "destination": {
      "branch": {
        "name": "main"
      }
    }
  },
  "repository": {
    "uuid": "{12345-67890}",
    "name": "test-repo",
    "full_name": "test-org/test-repo"
  },
  "actor": {
    "display_name": "Test User"
  }
}`)
}
