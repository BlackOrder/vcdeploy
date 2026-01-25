//go:build integration

// Package testutil provides shared testing utilities for vcdeploy.
package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// SSHContainer represents a test SSH server container.
type SSHContainer struct {
	Container testcontainers.Container
	Host      string
	Port      string
	User      string
	Password  string
}

// NewSSHContainer creates and starts an SSH server container for testing.
// It returns the container details and registers cleanup with t.Cleanup().
func NewSSHContainer(ctx context.Context, t *testing.T) (*SSHContainer, error) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "lscr.io/linuxserver/openssh-server:latest",
		ExposedPorts: []string{"2222/tcp"},
		Env: map[string]string{
			"PUID":            "1000",
			"PGID":            "1000",
			"TZ":              "UTC",
			"USER_NAME":       "testuser",
			"USER_PASSWORD":   "testpass",
			"PASSWORD_ACCESS": "true",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("sshd is listening").WithStartupTimeout(90*time.Second),
			wait.ForListeningPort("2222/tcp").WithStartupTimeout(90*time.Second),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start SSH container: %w", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate SSH container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "2222")
	if err != nil {
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	return &SSHContainer{
		Container: container,
		Host:      host,
		Port:      mappedPort.Port(),
		User:      "testuser",
		Password:  "testpass",
	}, nil
}

// Address returns the SSH address in host:port format.
func (s *SSHContainer) Address() string {
	return fmt.Sprintf("%s:%s", s.Host, s.Port)
}

// GitServerContainer represents a test Git server container.
type GitServerContainer struct {
	Container testcontainers.Container
	Host      string
	HTTPPort  string
	SSHPort   string
	AdminUser string
	AdminPass string
}

// NewGitServerContainer creates and starts a Gitea server for testing.
// It returns the container details and registers cleanup with t.Cleanup().
func NewGitServerContainer(ctx context.Context, t *testing.T) (*GitServerContainer, error) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "gitea/gitea:latest",
		ExposedPorts: []string{"3000/tcp", "22/tcp"},
		Env: map[string]string{
			"GITEA__security__INSTALL_LOCK": "true",
			"GITEA__server__ROOT_URL":       "http://localhost:3000",
			"USER_UID":                      "1000",
			"USER_GID":                      "1000",
		},
		WaitingFor: wait.ForHTTP("/").WithPort("3000/tcp").WithStartupTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start Git server container: %w", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate Git server container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	httpPort, err := container.MappedPort(ctx, "3000")
	if err != nil {
		return nil, fmt.Errorf("failed to get HTTP port: %w", err)
	}

	sshPort, err := container.MappedPort(ctx, "22")
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH port: %w", err)
	}

	return &GitServerContainer{
		Container: container,
		Host:      host,
		HTTPPort:  httpPort.Port(),
		SSHPort:   sshPort.Port(),
		AdminUser: "gitea_admin",
		AdminPass: "TestAdmin123!@#",
	}, nil
}

// HTTPURL returns the Git server HTTP URL.
func (g *GitServerContainer) HTTPURL() string {
	return fmt.Sprintf("http://%s:%s", g.Host, g.HTTPPort)
}

// SSHURL returns the Git server SSH URL.
func (g *GitServerContainer) SSHURL() string {
	return fmt.Sprintf("ssh://git@%s:%s", g.Host, g.SSHPort)
}

// WaitForReady waits for a container to be fully ready.
func WaitForReady(ctx context.Context, container testcontainers.Container, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for container to be ready")
		default:
			state, err := container.State(ctx)
			if err != nil {
				return fmt.Errorf("failed to get container state: %w", err)
			}
			if state.Running {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}
