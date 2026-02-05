// Package git provides repository management services for vcdeploy.
// It handles cloning private repositories using stored credentials
// and creating archives to stream to agents.
package git

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	// ErrNoCredential indicates no matching credential was found for the repository URL.
	ErrNoCredential = errors.New("no matching credential found")

	// ErrCloneFailed indicates the git clone operation failed.
	ErrCloneFailed = errors.New("git clone failed")
)

// RepoArchive represents a cloned repository packaged as a tar.gz archive.
type RepoArchive struct {
	// Path is the filesystem path to the archive file.
	Path string
	// Checksum is the SHA256 hash of the archive contents.
	Checksum string
	// Size is the archive file size in bytes.
	Size int64
}

// Service handles git operations for vcdeploy.
// It clones repositories using stored credentials and creates archives
// that can be streamed to agents without exposing credentials.
type Service struct {
	store   storage.Store
	kms     *security.KMS
	workDir string
	logger  *zap.Logger
}

// NewService creates a new git service.
func NewService(store storage.Store, kms *security.KMS, workDir string, logger *zap.Logger) *Service {
	return &Service{
		store:   store,
		kms:     kms,
		workDir: workDir,
		logger:  logger.Named("git"),
	}
}

// CloneAndArchive clones a repository and creates a tar.gz archive.
// The archive excludes the .git directory to minimize size.
// Credentials are retrieved from the store and never exposed to agents.
func (g *Service) CloneAndArchive(ctx context.Context, repoURL, ref string) (*RepoArchive, error) {
	g.logger.Info("Cloning repository",
		zap.String("url", sanitizeURL(repoURL)),
		zap.String("ref", ref),
	)

	// Ensure work directory exists
	// #nosec G301 - Work directory needs access for git operations
	if err := os.MkdirAll(g.workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}

	// Find matching credential
	cred, err := g.findCredential(ctx, repoURL)
	if err != nil && !errors.Is(err, ErrNoCredential) {
		return nil, fmt.Errorf("find credential: %w", err)
	}

	// Clone to temp directory
	tmpDir, err := os.MkdirTemp(g.workDir, "clone-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDir := filepath.Join(tmpDir, "repo")

	// Build and run git command with credential
	if err := g.runClone(ctx, repoURL, ref, cloneDir, cred); err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}

	// Remove .git directory (not needed on agent, saves bandwidth)
	gitDir := filepath.Join(cloneDir, ".git")
	if err := os.RemoveAll(gitDir); err != nil {
		g.logger.Warn("Failed to remove .git directory", zap.Error(err))
	}

	// Create tar.gz archive
	archiveID := uuid.New().String()
	archivePath := filepath.Join(g.workDir, fmt.Sprintf("repo-%s.tar.gz", archiveID))

	checksum, err := g.createArchive(cloneDir, archivePath)
	if err != nil {
		return nil, fmt.Errorf("create archive: %w", err)
	}

	// Get file size
	info, err := os.Stat(archivePath)
	if err != nil {
		os.Remove(archivePath)
		return nil, fmt.Errorf("stat archive: %w", err)
	}

	g.logger.Info("Created repository archive",
		zap.String("url", sanitizeURL(repoURL)),
		zap.String("archive", archivePath),
		zap.Int64("size", info.Size()),
		zap.String("checksum", checksum[:16]+"..."),
	)

	return &RepoArchive{
		Path:     archivePath,
		Checksum: checksum,
		Size:     info.Size(),
	}, nil
}

// decryptedCredential holds a credential with its decrypted value.
type decryptedCredential struct {
	*storage.SourceCredential
	Credential []byte // Decrypted credential value
}

// findCredential finds a stored credential that matches the repository URL.
func (g *Service) findCredential(ctx context.Context, repoURL string) (*decryptedCredential, error) {
	creds, err := g.store.ListSourceCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}

	for _, cred := range creds {
		if cred.URLPattern == "" {
			continue
		}

		pattern, err := regexp.Compile(cred.URLPattern)
		if err != nil {
			g.logger.Warn("Invalid credential URL pattern",
				zap.Int64("id", cred.ID),
				zap.String("pattern", cred.URLPattern),
				zap.Error(err),
			)
			continue
		}

		if pattern.MatchString(repoURL) {
			g.logger.Debug("Found matching credential",
				zap.Int64("id", cred.ID),
				zap.String("type", cred.Type),
			)

			result := &decryptedCredential{
				SourceCredential: cred,
			}

			// Decrypt credential if encrypted
			if g.kms != nil && len(cred.CredentialEnc) > 0 {
				decrypted, err := g.kms.Decrypt(ctx, string(cred.CredentialEnc))
				if err != nil {
					return nil, fmt.Errorf("decrypt credential: %w", err)
				}
				result.Credential = decrypted
			}

			return result, nil
		}
	}

	return nil, ErrNoCredential
}

// runClone executes the git clone command with appropriate credentials.
func (g *Service) runClone(ctx context.Context, repoURL, ref, destDir string, cred *decryptedCredential) error {
	args := []string{"clone", "--depth", "1", "--single-branch"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}

	env := os.Environ()
	cloneURL := repoURL

	// Handle different credential types
	if cred != nil {
		switch cred.Type {
		case "ssh_key":
			// Write SSH key to temp file
			keyFile, err := g.writeSSHKey(cred.Credential)
			if err != nil {
				return fmt.Errorf("write SSH key: %w", err)
			}
			defer os.Remove(keyFile)

			// Use GIT_SSH_COMMAND with key
			sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=accept-new -o IdentitiesOnly=yes", keyFile)
			env = append(env, "GIT_SSH_COMMAND="+sshCmd)

		case "https_token":
			// Embed token in URL
			cloneURL = embedTokenInURL(repoURL, string(cred.Credential))

		case "https_basic":
			// Parse username:password and embed in URL
			parts := strings.SplitN(string(cred.Credential), ":", 2)
			if len(parts) == 2 {
				cloneURL = embedBasicAuthInURL(repoURL, parts[0], parts[1])
			}
		}
	}

	args = append(args, cloneURL, destDir)

	g.logger.Debug("Running git clone",
		zap.String("dest", destDir),
		zap.String("ref", ref),
	)

	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: args are internally constructed
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Sanitize output to remove potential credentials
		sanitizedOutput := sanitizeOutput(string(output))
		g.logger.Error("Git clone failed",
			zap.String("output", sanitizedOutput),
			zap.Error(err),
		)
		return fmt.Errorf("%w: %s", ErrCloneFailed, sanitizedOutput)
	}

	return nil
}

// writeSSHKey writes an SSH key to a temporary file with proper permissions.
func (g *Service) writeSSHKey(key []byte) (string, error) {
	tmpFile, err := os.CreateTemp(g.workDir, "ssh-key-*")
	if err != nil {
		return "", err
	}

	if err := os.Chmod(tmpFile.Name(), 0o600); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	if _, err := tmpFile.Write(key); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// createArchive creates a tar.gz archive of the source directory.
// Returns the SHA256 checksum of the archive.
func (g *Service) createArchive(srcDir, destPath string) (string, error) {
	file, err := os.Create(destPath) // #nosec G304 - destPath is constructed from admin-controlled workDir
	if err != nil {
		return "", fmt.Errorf("create archive file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	mw := io.MultiWriter(file, hash)

	gw := gzip.NewWriter(mw)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Use relative path in archive
		header.Name = relPath

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			header.Linkname = link
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Write file contents (only for regular files)
		if info.Mode().IsRegular() {
			f, err := os.Open(path) // #nosec G304 - path is within admin-controlled srcDir from filepath.Walk
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("walk directory: %w", err)
	}

	// Close writers to flush
	if err := tw.Close(); err != nil {
		return "", fmt.Errorf("close tar: %w", err)
	}
	if err := gw.Close(); err != nil {
		return "", fmt.Errorf("close gzip: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// embedTokenInURL embeds a token into an HTTPS URL for authentication.
func embedTokenInURL(repoURL, token string) string {
	// Handle GitHub-style token URLs
	if strings.HasPrefix(repoURL, "https://github.com/") {
		return strings.Replace(repoURL, "https://github.com/", fmt.Sprintf("https://x-access-token:%s@github.com/", token), 1)
	}

	// Handle GitLab-style token URLs
	if strings.Contains(repoURL, "gitlab") {
		// GitLab uses oauth2:token format
		return strings.Replace(repoURL, "https://", fmt.Sprintf("https://oauth2:%s@", token), 1)
	}

	// Generic: insert token before host
	return strings.Replace(repoURL, "https://", fmt.Sprintf("https://%s@", token), 1)
}

// embedBasicAuthInURL embeds username:password into an HTTPS URL.
func embedBasicAuthInURL(repoURL, username, password string) string {
	return strings.Replace(repoURL, "https://", fmt.Sprintf("https://%s:%s@", username, password), 1)
}

// sanitizeURL removes credentials from a URL for logging.
func sanitizeURL(url string) string {
	// Remove credentials from URLs like https://user:pass@host/... or https://token@host/...
	re := regexp.MustCompile(`(https?://)[^@]+@`)
	return re.ReplaceAllString(url, "${1}")
}

// sanitizeOutput removes potential credentials from command output.
func sanitizeOutput(output string) string {
	// Remove token-like strings
	tokenRe := regexp.MustCompile(`(ghp_|glpat-|ghs_)[A-Za-z0-9_]+`)
	output = tokenRe.ReplaceAllString(output, "[REDACTED]")

	// Remove basic auth patterns
	authRe := regexp.MustCompile(`https://[^:]+:[^@]+@`)
	output = authRe.ReplaceAllString(output, "https://[REDACTED]@")

	return output
}
