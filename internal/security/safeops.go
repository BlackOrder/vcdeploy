// Package security provides safe file system operations with path traversal protection.
package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	// ErrOperationDenied is returned when a file operation is denied due to security restrictions.
	ErrOperationDenied = errors.New("operation denied: path is outside allowed base directory")

	// ErrSymlinkTarget is returned when a symlink target would escape the base directory.
	ErrSymlinkTarget = errors.New("symlink target escapes base directory")

	// ErrNotDirectory is returned when a directory operation is performed on a non-directory.
	ErrNotDirectory = errors.New("path is not a directory")

	// ErrRemoveProtected is returned when attempting to remove a protected path.
	ErrRemoveProtected = errors.New("cannot remove protected path")
)

// SafeOps provides safe file system operations restricted to a base directory.
type SafeOps struct {
	// BaseDir is the root directory that all operations are restricted to.
	BaseDir string
	// AllowSymlinks controls whether symlink creation is allowed.
	AllowSymlinks bool
}

// NewSafeOps creates a new SafeOps instance restricted to the given base directory.
func NewSafeOps(baseDir string) (*SafeOps, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("invalid base directory: %w", err)
	}

	// Ensure the base directory exists
	info, err := os.Stat(absBase)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("base directory does not exist: %s", absBase)
		}
		return nil, fmt.Errorf("cannot access base directory: %w", err)
	}
	if !info.IsDir() {
		return nil, ErrNotDirectory
	}

	return &SafeOps{
		BaseDir:       absBase,
		AllowSymlinks: false, // Disabled by default for security
	}, nil
}

// validatePath checks if a path is within the base directory.
// For relative paths, they are resolved relative to the base directory.
func (s *SafeOps) validatePath(path string) (string, error) {
	var absPath string
	var err error

	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		// Relative paths are resolved relative to base directory
		absPath, err = filepath.Abs(filepath.Join(s.BaseDir, path))
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}
	}

	if !IsWithinBase(s.BaseDir, absPath) {
		return "", ErrOperationDenied
	}

	return absPath, nil
}

// SafeRemove removes a file or empty directory within the base directory.
// It refuses to remove the base directory itself or any path outside it.
func (s *SafeOps) SafeRemove(path string) error {
	absPath, err := s.validatePath(path)
	if err != nil {
		return err
	}

	// Never allow removing the base directory itself
	if absPath == s.BaseDir {
		return ErrRemoveProtected
	}

	return os.Remove(absPath)
}

// SafeRemoveAll removes a path and any children within the base directory.
// It refuses to remove the base directory itself or any path outside it.
func (s *SafeOps) SafeRemoveAll(path string) error {
	absPath, err := s.validatePath(path)
	if err != nil {
		return err
	}

	// Never allow removing the base directory itself
	if absPath == s.BaseDir {
		return ErrRemoveProtected
	}

	return os.RemoveAll(absPath)
}

// SafeMkdir creates a directory within the base directory.
func (s *SafeOps) SafeMkdir(path string, perm os.FileMode) error {
	absPath, err := s.validatePath(path)
	if err != nil {
		// For mkdir, we need to check if the target WOULD be inside base
		// even if it doesn't exist yet
		joined, joinErr := SafeJoin(s.BaseDir, path)
		if joinErr != nil {
			return joinErr
		}
		absPath = joined
	}

	return os.Mkdir(absPath, perm)
}

// SafeMkdirAll creates a directory and all parent directories within the base directory.
func (s *SafeOps) SafeMkdirAll(path string, perm os.FileMode) error {
	// For paths that may not exist yet, use SafeJoin to validate
	var absPath string
	if filepath.IsAbs(path) {
		if !IsWithinBase(s.BaseDir, path) {
			return ErrOperationDenied
		}
		absPath = filepath.Clean(path)
	} else {
		joined, err := SafeJoin(s.BaseDir, path)
		if err != nil {
			return err
		}
		absPath = joined
	}

	return os.MkdirAll(absPath, perm)
}

// SafeSymlink creates a symbolic link within the base directory.
// Both the link path and the target must be within the base directory.
func (s *SafeOps) SafeSymlink(target, linkPath string) error {
	if !s.AllowSymlinks {
		return errors.New("symlink creation is disabled")
	}

	// Validate the link path
	absLinkPath, err := s.validatePath(linkPath)
	if err != nil {
		// If link doesn't exist yet, validate using SafeJoin
		if filepath.IsAbs(linkPath) {
			if !IsWithinBase(s.BaseDir, linkPath) {
				return ErrOperationDenied
			}
			absLinkPath = filepath.Clean(linkPath)
		} else {
			joined, joinErr := SafeJoin(s.BaseDir, linkPath)
			if joinErr != nil {
				return joinErr
			}
			absLinkPath = joined
		}
	}

	// Validate the target - it must also be within base directory
	// For relative targets, resolve them relative to the link's directory
	if filepath.IsAbs(target) {
		if !IsWithinBase(s.BaseDir, target) {
			return ErrSymlinkTarget
		}
	} else {
		// Resolve relative target from link's directory
		linkDir := filepath.Dir(absLinkPath)
		absTarget := filepath.Join(linkDir, target)
		if !IsWithinBase(s.BaseDir, absTarget) {
			return ErrSymlinkTarget
		}
	}

	return os.Symlink(target, absLinkPath)
}

// SafeRename renames/moves a file within the base directory.
// Both source and destination must be within the base directory.
func (s *SafeOps) SafeRename(oldPath, newPath string) error {
	absOld, err := s.validatePath(oldPath)
	if err != nil {
		return fmt.Errorf("source path: %w", err)
	}

	var absNew string
	if filepath.IsAbs(newPath) {
		if !IsWithinBase(s.BaseDir, newPath) {
			return fmt.Errorf("destination path: %w", ErrOperationDenied)
		}
		absNew = filepath.Clean(newPath)
	} else {
		joined, joinErr := SafeJoin(s.BaseDir, newPath)
		if joinErr != nil {
			return fmt.Errorf("destination path: %w", joinErr)
		}
		absNew = joined
	}

	return os.Rename(absOld, absNew)
}

// SafeWriteFile writes data to a file within the base directory.
func (s *SafeOps) SafeWriteFile(path string, data []byte, perm os.FileMode) error {
	var absPath string
	if filepath.IsAbs(path) {
		if !IsWithinBase(s.BaseDir, path) {
			return ErrOperationDenied
		}
		absPath = filepath.Clean(path)
	} else {
		joined, err := SafeJoin(s.BaseDir, path)
		if err != nil {
			return err
		}
		absPath = joined
	}

	return os.WriteFile(absPath, data, perm)
}

// SafeReadFile reads a file within the base directory.
func (s *SafeOps) SafeReadFile(path string) ([]byte, error) {
	absPath, err := s.validatePath(path)
	if err != nil {
		return nil, err
	}

	return os.ReadFile(absPath) // #nosec G304 - Path validated by validatePath before use
}

// SafeOpen opens a file within the base directory.
func (s *SafeOps) SafeOpen(path string) (*os.File, error) {
	absPath, err := s.validatePath(path)
	if err != nil {
		return nil, err
	}

	return os.Open(absPath) // #nosec G304 - Path validated by validatePath before use
}

// SafeCreate creates or truncates a file within the base directory.
func (s *SafeOps) SafeCreate(path string) (*os.File, error) {
	var absPath string
	if filepath.IsAbs(path) {
		if !IsWithinBase(s.BaseDir, path) {
			return nil, ErrOperationDenied
		}
		absPath = filepath.Clean(path)
	} else {
		joined, err := SafeJoin(s.BaseDir, path)
		if err != nil {
			return nil, err
		}
		absPath = joined
	}

	return os.Create(absPath) // #nosec G304 - Path validated by IsWithinBase/SafeJoin before use
}

// SafeStat returns file info for a path within the base directory.
func (s *SafeOps) SafeStat(path string) (os.FileInfo, error) {
	absPath, err := s.validatePath(path)
	if err != nil {
		return nil, err
	}

	return os.Stat(absPath)
}

// SafeLstat returns file info (without following symlinks) for a path within the base directory.
func (s *SafeOps) SafeLstat(path string) (os.FileInfo, error) {
	absPath, err := s.validatePath(path)
	if err != nil {
		return nil, err
	}

	return os.Lstat(absPath)
}

// SafeChmod changes the mode of a file within the base directory.
func (s *SafeOps) SafeChmod(path string, mode os.FileMode) error {
	absPath, err := s.validatePath(path)
	if err != nil {
		return err
	}

	return os.Chmod(absPath, mode)
}

// SafeChown changes the owner of a file within the base directory.
func (s *SafeOps) SafeChown(path string, uid, gid int) error {
	absPath, err := s.validatePath(path)
	if err != nil {
		return err
	}

	return os.Chown(absPath, uid, gid)
}
