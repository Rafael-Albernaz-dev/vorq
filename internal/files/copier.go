package files

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyFile copies a single file from src to dst, preserving file mode/permissions.
func CopyFile(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source file stat error: %w", err)
	}

	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("source %s is not a regular file", src)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to open destination file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return out.Sync()
}

// ValidateDeclared checks explicitly declared files before anything is
// created: each must be a workspace-relative path (no absolute path, "..",
// or .git metadata) naming an existing regular file, never a symlink.
func ValidateDeclared(srcDir string, extraFiles []string) error {
	for _, extra := range extraFiles {
		clean := filepath.Clean(extra)
		if extra == "" || filepath.IsAbs(extra) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("--copy %q: use a path relative to the workspace, without \"..\"", extra)
		}
		for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
			if part == ".git" {
				return fmt.Errorf("--copy %q: Git metadata cannot be copied", extra)
			}
		}
		root, err := os.OpenRoot(srcDir)
		if err != nil {
			return err
		}
		info, err := root.Lstat(clean)
		root.Close()
		if err != nil {
			return fmt.Errorf("--copy %q: file not found in the current workspace", extra)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("--copy %q: only regular files are copied (no symlinks or directories)", extra)
		}
	}
	return nil
}

// SyncLocalFiles copies unversioned configuration files from srcDir to dstDir.
// It tries copying .env.local and .env. If neither exists, it looks for .env.example
// and copies it as .env. Any extraFiles specified are also copied.
func SyncLocalFiles(srcDir, dstDir string, extraFiles []string) ([]string, error) {
	var copied []string

	copiedEnv := false

	// 1. Try .env.local
	envLocalSrc := filepath.Join(srcDir, ".env.local")
	if info, err := os.Stat(envLocalSrc); err == nil && info.Mode().IsRegular() {
		if err := CopyFile(envLocalSrc, filepath.Join(dstDir, ".env.local")); err != nil {
			return copied, fmt.Errorf("failed to copy .env.local: %w", err)
		}
		copied = append(copied, ".env.local")
		copiedEnv = true
	}

	// 2. Try .env
	envSrc := filepath.Join(srcDir, ".env")
	if info, err := os.Stat(envSrc); err == nil && info.Mode().IsRegular() {
		if err := CopyFile(envSrc, filepath.Join(dstDir, ".env")); err != nil {
			return copied, fmt.Errorf("failed to copy .env: %w", err)
		}
		copied = append(copied, ".env")
		copiedEnv = true
	}

	// 3. If neither .env nor .env.local existed, fallback to .env.example -> .env
	if !copiedEnv {
		exampleSrc := filepath.Join(srcDir, ".env.example")
		if info, err := os.Stat(exampleSrc); err == nil && info.Mode().IsRegular() {
			if err := CopyFile(exampleSrc, filepath.Join(dstDir, ".env")); err != nil {
				return copied, fmt.Errorf("failed to copy .env.example: %w", err)
			}
			copied = append(copied, ".env (from .env.example)")
		}
	}

	// 4. Copy explicitly declared extra files (validated: never outside the
	// workspaces, never silently skipped).
	if err := ValidateDeclared(srcDir, extraFiles); err != nil {
		return copied, err
	}
	for _, extra := range extraFiles {
		clean := filepath.Clean(extra)
		if err := CopyFile(filepath.Join(srcDir, clean), filepath.Join(dstDir, clean)); err != nil {
			return copied, fmt.Errorf("failed to copy %s: %w", extra, err)
		}
		copied = append(copied, extra)
	}

	return copied, nil
}
