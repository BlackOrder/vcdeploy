// Package deploy provides deployment execution logic.
package deploy

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/validation"
)

// Executor defines the interface for deployment execution.
type Executor interface {
	// Execute runs a deployment on the target.
	Execute(ctx context.Context, cmd *DeployCommand) (*DeployResult, error)

	// Rollback rolls back to a previous release.
	Rollback(ctx context.Context, cmd *RollbackCommand) (*DeployResult, error)

	// StreamLogs returns a channel for streaming deployment logs.
	StreamLogs(ctx context.Context, deploymentID string) (<-chan LogEntry, error)

	// CheckHealth verifies the deployment is healthy.
	CheckHealth(ctx context.Context, url string, timeout time.Duration, retries int) error

	// Close releases any resources.
	Close() error
}

// DeployCommand contains all information needed for a deployment.
type DeployCommand struct {
	DeploymentID    string
	Project         string
	Target          string
	Repository      string
	Branch          string
	Commit          string
	Path            string
	Settings        DeploySettings
	EnvVars         map[string]string
	EnvFileContent  []byte
	PreDeployHooks  []string
	PostDeployHooks []string
	ReloadServices  []ServiceReload
}

// DeploySettings contains deployment configuration.
type DeploySettings struct {
	Strategy       string // symlink | inplace
	KeepReleases   int
	SharedDirs     []string
	SharedFiles    []string
	WritableDirs   []string
	ExecutionUser  string
	ExecutionGroup string
	Timeout        time.Duration
}

// ServiceReload defines a service to reload.
type ServiceReload struct {
	Service string
	Action  string // reload | restart | start | stop
}

// RollbackCommand contains information for a rollback.
type RollbackCommand struct {
	DeploymentID   string
	Project        string
	Target         string
	Path           string
	ReleaseNumber  int // 0 = previous release
	RollbackHooks  []string
	ReloadServices []ServiceReload
}

// DeployResult contains the result of a deployment.
type DeployResult struct {
	Success       bool
	ReleaseNumber int
	ReleasePath   string
	StartedAt     time.Time
	CompletedAt   time.Time
	Error         error
	Logs          []LogEntry
}

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	Message   string
	Source    string
}

// LogLevel represents log severity.
type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

func (l LogLevel) String() string {
	switch l {
	case LogDebug:
		return "DEBUG"
	case LogInfo:
		return "INFO"
	case LogWarn:
		return "WARN"
	case LogError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogWriter wraps log output for a deployment.
type LogWriter struct {
	DeploymentID string
	Source       string
	Level        LogLevel
	Output       chan<- LogEntry
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	w.Output <- LogEntry{
		Timestamp: time.Now(),
		Level:     w.Level,
		Message:   string(p),
		Source:    w.Source,
	}
	return len(p), nil
}

// CommandRunner executes shell commands.
type CommandRunner interface {
	// Run executes a command and returns output.
	Run(ctx context.Context, cmd string, opts RunOptions) (*CommandResult, error)

	// RunWithOutput executes a command with streaming output.
	RunWithOutput(ctx context.Context, cmd string, stdout, stderr io.Writer, opts RunOptions) error
}

// RunOptions contains options for running commands.
type RunOptions struct {
	WorkDir string
	Env     map[string]string
	User    string
	Group   string
	Timeout time.Duration
}

// CommandResult contains the result of a command execution.
type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// --- Symlink deployment strategy ---

// SymlinkStrategy implements symlink-based zero-downtime deployments.
type SymlinkStrategy struct {
	runner CommandRunner
}

// NewSymlinkStrategy creates a new symlink deployment strategy.
func NewSymlinkStrategy(runner CommandRunner) *SymlinkStrategy {
	return &SymlinkStrategy{runner: runner}
}

// Deploy performs a symlink-based deployment.
func (s *SymlinkStrategy) Deploy(ctx context.Context, cmd *DeployCommand, logCh chan<- LogEntry) (*DeployResult, error) {
	result := &DeployResult{
		StartedAt: time.Now(),
	}

	log := func(level LogLevel, msg string, args ...interface{}) {
		logCh <- LogEntry{
			Timestamp: time.Now(),
			Level:     level,
			Message:   fmt.Sprintf(msg, args...),
			Source:    "deploy",
		}
	}

	basePath := cmd.Path
	releasesPath := basePath + "/releases"
	sharedPath := basePath + "/shared"
	currentPath := basePath + "/current"
	repoPath := basePath + "/repo"

	// Get next release number
	releaseNum, err := s.getNextReleaseNumber(ctx, releasesPath)
	if err != nil {
		result.Error = fmt.Errorf("getting release number: %w", err)
		return result, result.Error
	}
	result.ReleaseNumber = releaseNum
	releasePath := fmt.Sprintf("%s/%d", releasesPath, releaseNum)
	result.ReleasePath = releasePath

	log(LogInfo, "Starting deployment #%d", releaseNum)

	// 1. Setup directories
	log(LogInfo, "Setting up directories...")
	if err := s.setupDirectories(ctx, basePath, releasesPath, sharedPath, cmd.Settings); err != nil {
		result.Error = fmt.Errorf("setup directories: %w", err)
		return result, result.Error
	}

	// 2. Clone/update repository
	log(LogInfo, "Updating repository...")
	if err := s.updateRepo(ctx, repoPath, cmd.Repository, cmd.Branch, cmd.Commit, logCh); err != nil {
		result.Error = fmt.Errorf("update repo: %w", err)
		return result, result.Error
	}

	// 3. Create release directory from repo
	log(LogInfo, "Creating release #%d...", releaseNum)
	if err := s.createRelease(ctx, repoPath, releasePath, cmd.Commit); err != nil {
		result.Error = fmt.Errorf("create release: %w", err)
		return result, result.Error
	}

	// 4. Link shared files and directories
	log(LogInfo, "Linking shared resources...")
	if err := s.linkShared(ctx, releasePath, sharedPath, cmd.Settings); err != nil {
		result.Error = fmt.Errorf("link shared: %w", err)
		return result, result.Error
	}

	// 5. Write env file
	if len(cmd.EnvFileContent) > 0 {
		log(LogInfo, "Writing environment file...")
		if err := s.writeEnvFile(ctx, sharedPath, cmd.EnvFileContent); err != nil {
			result.Error = fmt.Errorf("write env: %w", err)
			return result, result.Error
		}
	}

	// 6. Run pre-deploy hooks
	if len(cmd.PreDeployHooks) > 0 {
		log(LogInfo, "Running pre-deploy hooks...")
		if err := s.runHooks(ctx, releasePath, cmd.PreDeployHooks, cmd.Settings, logCh); err != nil {
			result.Error = fmt.Errorf("pre-deploy hooks: %w", err)
			return result, result.Error
		}
	}

	// 7. Run post-deploy hooks
	if len(cmd.PostDeployHooks) > 0 {
		log(LogInfo, "Running post-deploy hooks...")
		if err := s.runHooks(ctx, releasePath, cmd.PostDeployHooks, cmd.Settings, logCh); err != nil {
			result.Error = fmt.Errorf("post-deploy hooks: %w", err)
			return result, result.Error
		}
	}

	// 8. Atomic symlink swap
	log(LogInfo, "Activating release #%d...", releaseNum)
	if err := s.activateRelease(ctx, releasePath, currentPath); err != nil {
		result.Error = fmt.Errorf("activate release: %w", err)
		return result, result.Error
	}

	// 9. Reload services
	if len(cmd.ReloadServices) > 0 {
		log(LogInfo, "Reloading services...")
		if err := s.reloadServices(ctx, cmd.ReloadServices, logCh); err != nil {
			result.Error = fmt.Errorf("reload services: %w", err)
			return result, result.Error
		}
	}

	// 10. Cleanup old releases
	log(LogInfo, "Cleaning up old releases...")
	if err := s.cleanupReleases(ctx, releasesPath, cmd.Settings.KeepReleases); err != nil {
		log(LogWarn, "Cleanup warning: %v", err)
		// Don't fail deployment for cleanup errors
	}

	result.Success = true
	result.CompletedAt = time.Now()
	log(LogInfo, "Deployment #%d completed successfully in %v", releaseNum, result.CompletedAt.Sub(result.StartedAt))

	return result, nil
}

func (s *SymlinkStrategy) getNextReleaseNumber(ctx context.Context, releasesPath string) (int, error) {
	result, err := s.runner.Run(ctx, fmt.Sprintf("ls -1 %s 2>/dev/null | sort -n | tail -1", releasesPath), RunOptions{})
	if err != nil {
		return 1, nil // First release
	}
	var lastRelease int
	_, _ = fmt.Sscanf(result.Stdout, "%d", &lastRelease)
	return lastRelease + 1, nil
}

func (s *SymlinkStrategy) setupDirectories(ctx context.Context, basePath, releasesPath, sharedPath string, settings DeploySettings) error {
	dirs := []string{basePath, releasesPath, sharedPath}
	for _, dir := range settings.SharedDirs {
		dirs = append(dirs, sharedPath+"/"+dir)
	}

	for _, dir := range dirs {
		_, err := s.runner.Run(ctx, fmt.Sprintf("mkdir -p %s", dir), RunOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SymlinkStrategy) updateRepo(ctx context.Context, repoPath, repository, branch, commit string, logCh chan<- LogEntry) error {
	// Check if repo exists
	result, _ := s.runner.Run(ctx, fmt.Sprintf("test -d %s/.git", repoPath), RunOptions{})

	if result == nil || result.ExitCode != 0 {
		// Clone
		_, err := s.runner.Run(ctx, fmt.Sprintf("git clone --mirror %s %s", repository, repoPath), RunOptions{})
		if err != nil {
			return fmt.Errorf("git clone: %w", err)
		}
	} else {
		// Fetch
		_, err := s.runner.Run(ctx, fmt.Sprintf("git -C %s fetch --all --prune", repoPath), RunOptions{})
		if err != nil {
			return fmt.Errorf("git fetch: %w", err)
		}
	}
	return nil
}

func (s *SymlinkStrategy) createRelease(ctx context.Context, repoPath, releasePath, commit string) error {
	// Create release directory
	_, err := s.runner.Run(ctx, fmt.Sprintf("mkdir -p %s", releasePath), RunOptions{})
	if err != nil {
		return err
	}

	// Archive and extract from repo
	ref := commit
	if ref == "" {
		ref = "HEAD"
	}
	_, err = s.runner.Run(ctx, fmt.Sprintf("git -C %s archive %s | tar -x -C %s", repoPath, ref, releasePath), RunOptions{})
	return err
}

func (s *SymlinkStrategy) linkShared(ctx context.Context, releasePath, sharedPath string, settings DeploySettings) error {
	// Link shared directories
	for _, dir := range settings.SharedDirs {
		target := releasePath + "/" + dir
		source := sharedPath + "/" + dir

		// Remove existing directory in release
		_, _ = s.runner.Run(ctx, fmt.Sprintf("rm -rf %s", target), RunOptions{})

		// Create symlink
		_, err := s.runner.Run(ctx, fmt.Sprintf("ln -nfs %s %s", source, target), RunOptions{})
		if err != nil {
			return fmt.Errorf("link dir %s: %w", dir, err)
		}
	}

	// Link shared files
	for _, file := range settings.SharedFiles {
		target := releasePath + "/" + file
		source := sharedPath + "/" + file

		// Ensure parent directory exists
		_, _ = s.runner.Run(ctx, fmt.Sprintf("mkdir -p $(dirname %s)", target), RunOptions{})

		// Remove existing file in release
		_, _ = s.runner.Run(ctx, fmt.Sprintf("rm -f %s", target), RunOptions{})

		// Create symlink
		_, err := s.runner.Run(ctx, fmt.Sprintf("ln -nfs %s %s", source, target), RunOptions{})
		if err != nil {
			return fmt.Errorf("link file %s: %w", file, err)
		}
	}

	// Set writable permissions
	for _, dir := range settings.WritableDirs {
		target := releasePath + "/" + dir
		_, _ = s.runner.Run(ctx, fmt.Sprintf("mkdir -p %s && chmod -R 775 %s", target, target), RunOptions{})
	}

	return nil
}

func (s *SymlinkStrategy) writeEnvFile(ctx context.Context, sharedPath string, content []byte) error {
	if len(content) == 0 {
		return nil
	}

	envPath := sharedPath + "/.env"

	// Use base64 encoding to safely pass content through shell
	// This avoids issues with special characters in env file content
	encoded := base64.StdEncoding.EncodeToString(content)

	// Write the env file using echo with base64 decode
	// This is safe for any content including newlines and special chars
	cmd := fmt.Sprintf("echo '%s' | base64 -d > %s && chmod 600 %s", encoded, envPath, envPath)
	_, err := s.runner.Run(ctx, cmd, RunOptions{})
	if err != nil {
		return fmt.Errorf("write env file: %w", err)
	}

	return nil
}

func (s *SymlinkStrategy) runHooks(ctx context.Context, releasePath string, hooks []string, settings DeploySettings, logCh chan<- LogEntry) error {
	opts := RunOptions{
		WorkDir: releasePath,
		User:    settings.ExecutionUser,
		Group:   settings.ExecutionGroup,
		Timeout: settings.Timeout,
	}

	for _, hook := range hooks {
		logCh <- LogEntry{
			Timestamp: time.Now(),
			Level:     LogInfo,
			Message:   fmt.Sprintf("Running: %s", hook),
			Source:    "hook",
		}

		result, err := s.runner.Run(ctx, hook, opts)
		if err != nil {
			return fmt.Errorf("hook '%s' failed: %w", hook, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("hook '%s' exited with code %d: %s", hook, result.ExitCode, result.Stderr)
		}

		if result.Stdout != "" {
			logCh <- LogEntry{
				Timestamp: time.Now(),
				Level:     LogInfo,
				Message:   result.Stdout,
				Source:    "hook",
			}
		}
	}

	return nil
}

func (s *SymlinkStrategy) activateRelease(ctx context.Context, releasePath, currentPath string) error {
	// Atomic symlink swap using mv -T
	tmpLink := currentPath + ".tmp"

	// Create new symlink
	_, err := s.runner.Run(ctx, fmt.Sprintf("ln -nfs %s %s", releasePath, tmpLink), RunOptions{})
	if err != nil {
		return fmt.Errorf("create temp symlink: %w", err)
	}

	// Atomic move
	_, err = s.runner.Run(ctx, fmt.Sprintf("mv -T %s %s", tmpLink, currentPath), RunOptions{})
	if err != nil {
		// Fallback for systems without mv -T
		_, _ = s.runner.Run(ctx, fmt.Sprintf("rm -f %s", tmpLink), RunOptions{})
		_, err = s.runner.Run(ctx, fmt.Sprintf("ln -nfs %s %s", releasePath, currentPath), RunOptions{})
		if err != nil {
			return fmt.Errorf("activate symlink: %w", err)
		}
	}

	return nil
}

func (s *SymlinkStrategy) reloadServices(ctx context.Context, services []ServiceReload, logCh chan<- LogEntry) error {
	for _, svc := range services {
		// Validate service name to prevent command injection
		if !validation.IsValidServiceName(svc.Service) {
			return fmt.Errorf("invalid service name: %q", svc.Service)
		}

		logCh <- LogEntry{
			Timestamp: time.Now(),
			Level:     LogInfo,
			Message:   fmt.Sprintf("Service %s: %s", svc.Service, svc.Action),
			Source:    "service",
		}

		var cmd string
		switch svc.Action {
		case "reload":
			cmd = fmt.Sprintf("systemctl reload %s || systemctl restart %s", svc.Service, svc.Service)
		case "restart":
			cmd = fmt.Sprintf("systemctl restart %s", svc.Service)
		case "start":
			cmd = fmt.Sprintf("systemctl start %s", svc.Service)
		case "stop":
			cmd = fmt.Sprintf("systemctl stop %s", svc.Service)
		default:
			return fmt.Errorf("unknown service action: %s", svc.Action)
		}

		_, err := s.runner.Run(ctx, cmd, RunOptions{})
		if err != nil {
			return fmt.Errorf("service %s %s: %w", svc.Service, svc.Action, err)
		}
	}
	return nil
}

func (s *SymlinkStrategy) cleanupReleases(ctx context.Context, releasesPath string, keepReleases int) error {
	if keepReleases <= 0 {
		keepReleases = 5
	}

	// List releases, sort by number, remove all but last N
	cmd := fmt.Sprintf("ls -1 %s | sort -n | head -n -%d | xargs -I {} rm -rf %s/{}",
		releasesPath, keepReleases, releasesPath)
	_, err := s.runner.Run(ctx, cmd, RunOptions{})
	return err
}

// Rollback rolls back to a previous release.
func (s *SymlinkStrategy) Rollback(ctx context.Context, cmd *RollbackCommand) (*DeployResult, error) {
	result := &DeployResult{
		StartedAt: time.Now(),
	}

	basePath := cmd.Path
	releasesPath := basePath + "/releases"
	currentPath := basePath + "/current"

	// List available releases
	listCmd := fmt.Sprintf("ls -1 %s | sort -n", releasesPath)
	listResult, err := s.runner.Run(ctx, listCmd, RunOptions{})
	if err != nil {
		result.Error = fmt.Errorf("listing releases: %w", err)
		return result, result.Error
	}

	releases := strings.Split(strings.TrimSpace(listResult.Stdout), "\n")
	if len(releases) < 2 {
		result.Error = fmt.Errorf("no previous release to rollback to")
		return result, result.Error
	}

	// Determine target release
	var targetRelease string
	if cmd.ReleaseNumber > 0 {
		// Specific release requested
		targetRelease = fmt.Sprintf("%d", cmd.ReleaseNumber)
		found := false
		for _, r := range releases {
			if r == targetRelease {
				found = true
				break
			}
		}
		if !found {
			result.Error = fmt.Errorf("release %d not found", cmd.ReleaseNumber)
			return result, result.Error
		}
	} else {
		// Roll back to previous release (second to last)
		targetRelease = releases[len(releases)-2]
	}

	releasePath := releasesPath + "/" + targetRelease
	result.ReleasePath = releasePath

	// Run rollback hooks
	for _, hook := range cmd.RollbackHooks {
		hookCmd := fmt.Sprintf("cd %s && %s", releasePath, hook)
		_, err := s.runner.Run(ctx, hookCmd, RunOptions{
			WorkDir: releasePath,
		})
		if err != nil {
			result.Error = fmt.Errorf("running rollback hook: %w", err)
			return result, result.Error
		}
	}

	// Atomically switch the symlink
	tmpLink := currentPath + ".tmp"

	// Create new symlink
	_, err = s.runner.Run(ctx, fmt.Sprintf("ln -sfn %s %s", releasePath, tmpLink), RunOptions{})
	if err != nil {
		result.Error = fmt.Errorf("creating temp symlink: %w", err)
		return result, result.Error
	}

	// Atomic rename
	_, err = s.runner.Run(ctx, fmt.Sprintf("mv -Tf %s %s", tmpLink, currentPath), RunOptions{})
	if err != nil {
		result.Error = fmt.Errorf("activating release: %w", err)
		return result, result.Error
	}

	// Reload services
	for _, svc := range cmd.ReloadServices {
		svcCmd := fmt.Sprintf("systemctl %s %s", svc.Action, svc.Service)
		_, err := s.runner.Run(ctx, svcCmd, RunOptions{})
		if err != nil {
			result.Error = fmt.Errorf("service %s %s: %w", svc.Service, svc.Action, err)
			return result, result.Error
		}
	}

	result.Success = true
	result.CompletedAt = time.Now()
	return result, nil
}
