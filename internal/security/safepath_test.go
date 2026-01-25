package security

import (
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	tests := []struct {
		name         string
		base         string
		relativePath string
		want         string
		wantErr      error
	}{
		{
			name:         "simple join",
			base:         "/var/deploy",
			relativePath: "app/config.yml",
			want:         "/var/deploy/app/config.yml",
			wantErr:      nil,
		},
		{
			name:         "join with dots in filename",
			base:         "/var/deploy",
			relativePath: "app/config.prod.yml",
			want:         "/var/deploy/app/config.prod.yml",
			wantErr:      nil,
		},
		{
			name:         "traversal attack blocked",
			base:         "/var/deploy",
			relativePath: "../etc/passwd",
			want:         "",
			wantErr:      ErrPathTraversal,
		},
		{
			name:         "deep traversal attack blocked",
			base:         "/var/deploy",
			relativePath: "app/../../etc/passwd",
			want:         "",
			wantErr:      ErrPathTraversal,
		},
		{
			name:         "absolute path blocked",
			base:         "/var/deploy",
			relativePath: "/etc/passwd",
			want:         "",
			wantErr:      ErrAbsolutePath,
		},
		{
			name:         "empty base path",
			base:         "",
			relativePath: "app/config.yml",
			want:         "",
			wantErr:      ErrEmptyPath,
		},
		{
			name:         "empty relative path",
			base:         "/var/deploy",
			relativePath: "",
			want:         "",
			wantErr:      ErrEmptyPath,
		},
		{
			name:         "current directory reference allowed",
			base:         "/var/deploy",
			relativePath: "./app/config.yml",
			want:         "/var/deploy/app/config.yml",
			wantErr:      nil,
		},
		{
			name:         "nested traversal blocked",
			base:         "/var/deploy",
			relativePath: "app/logs/../../../etc/passwd",
			want:         "",
			wantErr:      ErrPathTraversal,
		},
		{
			name:         "valid nested path",
			base:         "/var/deploy",
			relativePath: "app/logs/../config/app.yml",
			want:         "/var/deploy/app/config/app.yml",
			wantErr:      nil,
		},
		{
			name:         "just base directory",
			base:         "/var/deploy",
			relativePath: ".",
			want:         "/var/deploy",
			wantErr:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeJoin(tt.base, tt.relativePath)
			if err != tt.wantErr {
				t.Errorf("SafeJoin() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SafeJoin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsWithinBase(t *testing.T) {
	tests := []struct {
		name string
		base string
		path string
		want bool
	}{
		{
			name: "path within base",
			base: "/var/deploy",
			path: "/var/deploy/app/config.yml",
			want: true,
		},
		{
			name: "path is base",
			base: "/var/deploy",
			path: "/var/deploy",
			want: true,
		},
		{
			name: "path outside base",
			base: "/var/deploy",
			path: "/etc/passwd",
			want: false,
		},
		{
			name: "path with similar prefix but outside",
			base: "/var/deploy",
			path: "/var/deploy-other/app",
			want: false,
		},
		{
			name: "traversal path",
			base: "/var/deploy",
			path: "/var/deploy/../etc/passwd",
			want: false,
		},
		{
			name: "empty base",
			base: "",
			path: "/var/deploy/app",
			want: false,
		},
		{
			name: "empty path",
			base: "/var/deploy",
			path: "",
			want: false,
		},
		{
			name: "path with cleaned traversal staying inside",
			base: "/var/deploy",
			path: "/var/deploy/app/../config",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsWithinBase(tt.base, tt.path); got != tt.want {
				t.Errorf("IsWithinBase() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateRelativePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{
			name:    "simple relative path",
			path:    "app/config.yml",
			wantErr: nil,
		},
		{
			name:    "current directory",
			path:    "./app/config.yml",
			wantErr: nil,
		},
		{
			name:    "absolute path",
			path:    "/etc/passwd",
			wantErr: ErrAbsolutePath,
		},
		{
			name:    "traversal",
			path:    "../etc/passwd",
			wantErr: ErrPathTraversal,
		},
		{
			name:    "hidden traversal",
			path:    "app/../../etc/passwd",
			wantErr: ErrPathTraversal,
		},
		{
			name:    "just parent directory",
			path:    "..",
			wantErr: ErrPathTraversal,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: ErrEmptyPath,
		},
		{
			name:    "valid path with parent in middle that stays inside",
			path:    "app/../config",
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRelativePath(tt.path)
			if err != tt.wantErr {
				t.Errorf("ValidateRelativePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "clean path unchanged",
			path: "app/config.yml",
			want: "app/config.yml",
		},
		{
			name: "removes trailing separator",
			path: "app/config/",
			want: "app/config",
		},
		{
			name: "removes current directory",
			path: "./app/./config",
			want: "app/config",
		},
		{
			name: "removes null bytes",
			path: "app\x00/config",
			want: "app/config",
		},
		{
			name: "normalizes separators",
			path: filepath.FromSlash("app//config"),
			want: "app/config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizePath(tt.path); got != tt.want {
				t.Errorf("SanitizePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
