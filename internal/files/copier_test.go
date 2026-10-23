package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/files"
)

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "sub", "dest.txt")

	content := []byte("SECRET_KEY=12345\n")
	if err := os.WriteFile(src, content, 0600); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	if err := files.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("expected %q, got %q", string(content), string(got))
	}

	// Verify permissions
	dstInfo, _ := os.Stat(dst)
	if dstInfo.Mode().Perm() != 0600 {
		t.Errorf("expected mode 0600, got %v", dstInfo.Mode().Perm())
	}
}

func TestSyncLocalFiles_EnvFallback(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Only .env.example exists
	exampleFile := filepath.Join(srcDir, ".env.example")
	if err := os.WriteFile(exampleFile, []byte("PORT=3000\n"), 0644); err != nil {
		t.Fatalf("failed to write example: %v", err)
	}

	copied, err := files.SyncLocalFiles(srcDir, dstDir, nil)
	if err != nil {
		t.Fatalf("SyncLocalFiles failed: %v", err)
	}

	if len(copied) != 1 || copied[0] != ".env (from .env.example)" {
		t.Errorf("expected fallback to .env.example, got: %v", copied)
	}

	// Check that .env exists in dstDir
	if _, err := os.Stat(filepath.Join(dstDir, ".env")); err != nil {
		t.Errorf(".env does not exist in destination: %v", err)
	}
}

func TestSyncLocalFiles_EnvAndEnvLocal(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	_ = os.WriteFile(filepath.Join(srcDir, ".env"), []byte("BASE=1\n"), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, ".env.local"), []byte("LOCAL=2\n"), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "custom.conf"), []byte("EXTRA=3\n"), 0644)

	copied, err := files.SyncLocalFiles(srcDir, dstDir, []string{"custom.conf"})
	if err != nil {
		t.Fatalf("SyncLocalFiles failed: %v", err)
	}

	if len(copied) != 3 {
		t.Errorf("expected 3 files copied, got %v", copied)
	}
}
