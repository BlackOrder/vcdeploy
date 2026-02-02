// Package agent provides the agent daemon implementation.
package agent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/BlackOrder/vcdeploy/internal/proto"
	"go.uber.org/zap"
)

const (
	// maxFileSize is the maximum size for a single file (1GB)
	maxFileSize = 1 << 30
	// maxTotalSize is the maximum total extraction size (5GB)
	maxTotalSize = 5 << 30
	// maxFileCount is the maximum number of files in an archive
	maxFileCount = 100000
)

// receiveRepoArchive streams a repository archive from the master and extracts it.
// Returns the path to the extracted directory.
func (a *Agent) receiveRepoArchive(ctx context.Context, deploymentID, repoURL, ref string) (string, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return "", fmt.Errorf("not connected to master")
	}

	stream, err := client.StreamRepoArchive(ctx, &pb.StreamRepoRequest{
		DeploymentId: deploymentID,
		RepoUrl:      repoURL,
		Ref:          ref,
	})
	if err != nil {
		return "", fmt.Errorf("stream repo: %w", err)
	}

	// Create temp file for archive
	tmpDir := filepath.Join(a.config.Paths.Data, "tmp")
	if err := os.MkdirAll(tmpDir, 0750); err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(tmpDir, "repo-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Ensure cleanup on error
	success := false
	defer func() {
		tmpFile.Close()
		if !success {
			os.Remove(tmpPath)
		}
	}()

	hash := sha256.New()
	mw := io.MultiWriter(tmpFile, hash)

	var expectedChecksum string
	var totalSize int64
	var receivedSize int64

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("receive chunk: %w", err)
		}

		if chunk.IsLast {
			expectedChecksum = chunk.Checksum
			totalSize = chunk.TotalSize
			break
		}

		n, err := mw.Write(chunk.Data)
		if err != nil {
			return "", fmt.Errorf("write chunk: %w", err)
		}
		receivedSize += int64(n)

		// Enforce total size limit during streaming
		if receivedSize > maxTotalSize {
			return "", fmt.Errorf("archive size exceeds maximum allowed (%d bytes)", maxTotalSize)
		}
	}

	// Verify checksum
	actualChecksum := hex.EncodeToString(hash.Sum(nil))
	if actualChecksum != expectedChecksum {
		return "", fmt.Errorf("checksum mismatch: got %s, expected %s", actualChecksum, expectedChecksum)
	}

	a.logger.Info("Archive received and verified",
		zap.String("deployment_id", deploymentID),
		zap.Int64("size", totalSize),
		zap.String("checksum", actualChecksum[:16]+"..."))

	// Extract archive
	extractDir := filepath.Join(a.config.Paths.Data, "repos", deploymentID)
	if err := os.RemoveAll(extractDir); err != nil {
		a.logger.Warn("Failed to clean existing extract dir", zap.Error(err))
	}
	if err := os.MkdirAll(extractDir, 0750); err != nil {
		return "", fmt.Errorf("create extract dir: %w", err)
	}

	if err := a.extractArchive(tmpPath, extractDir); err != nil {
		os.RemoveAll(extractDir)
		return "", fmt.Errorf("extract archive: %w", err)
	}

	success = true
	os.Remove(tmpPath) // Clean up temp file after successful extraction

	return extractDir, nil
}

// extractArchive extracts a tar.gz archive to the destination directory.
// It includes security protections against path traversal, symlinks, and resource exhaustion.
func (a *Agent) extractArchive(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	// Resolve destination to absolute path for comparison
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve dest path: %w", err)
	}

	var totalExtracted int64
	var fileCount int

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		fileCount++
		if fileCount > maxFileCount {
			return fmt.Errorf("too many files in archive (max %d)", maxFileCount)
		}

		// Clean the path and join with destination
		cleanName := filepath.Clean(header.Name)
		if cleanName == "." {
			continue
		}

		// Reject absolute paths
		if filepath.IsAbs(cleanName) {
			return fmt.Errorf("absolute path not allowed: %s", header.Name)
		}

		// Reject paths with ..
		if strings.Contains(cleanName, "..") {
			return fmt.Errorf("path traversal not allowed: %s", header.Name)
		}

		target := filepath.Join(destDir, cleanName)

		// Resolve to absolute path
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("resolve target path: %w", err)
		}

		// Path traversal protection (double-check after resolution)
		if !strings.HasPrefix(targetAbs, destAbs+string(filepath.Separator)) && targetAbs != destAbs {
			return fmt.Errorf("path traversal detected: %s resolves outside dest", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0750); err != nil {
				return fmt.Errorf("create directory %s: %w", cleanName, err)
			}

		case tar.TypeReg:
			// Check file size before extraction
			if header.Size > maxFileSize {
				return fmt.Errorf("file too large: %s (%d bytes, max %d)", cleanName, header.Size, maxFileSize)
			}

			totalExtracted += header.Size
			if totalExtracted > maxTotalSize {
				return fmt.Errorf("total extraction size exceeds limit (%d bytes)", maxTotalSize)
			}

			if err := a.extractFile(tr, target, header); err != nil {
				return fmt.Errorf("extract file %s: %w", cleanName, err)
			}

		case tar.TypeSymlink, tar.TypeLink:
			// Reject symlinks and hard links for security
			return fmt.Errorf("symlinks and hard links not allowed: %s", header.Name)

		default:
			// Skip other types (devices, fifos, etc.)
			a.logger.Debug("Skipping unsupported file type",
				zap.String("name", header.Name),
				zap.Int("type", int(header.Typeflag)))
		}
	}

	a.logger.Info("Archive extracted successfully",
		zap.String("dest", destDir),
		zap.Int("files", fileCount),
		zap.Int64("bytes", totalExtracted))

	return nil
}

// extractFile extracts a single regular file from the tar reader.
func (a *Agent) extractFile(tr *tar.Reader, target string, header *tar.Header) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	// Sanitize file mode (remove setuid, setgid, sticky bits)
	mode := os.FileMode(header.Mode) & 0755
	if mode == 0 {
		mode = 0644
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// Use LimitReader to enforce size limit
	written, err := io.Copy(f, io.LimitReader(tr, maxFileSize+1))
	if err != nil {
		os.Remove(target)
		return fmt.Errorf("write file: %w", err)
	}
	if written > maxFileSize {
		os.Remove(target)
		return fmt.Errorf("file exceeded size limit during extraction")
	}

	return nil
}
