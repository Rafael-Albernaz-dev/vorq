package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

type failingInspection struct {
	app.Repository
	path, mode string
	reads      atomic.Int32
	removes    atomic.Int32
	cause      error
}

func (r *failingInspection) ReadChanges(ctx context.Context, dir, branch string) (*domain.ChangeSet, error) {
	if dir == r.path {
		count := r.reads.Add(1)
		switch r.mode {
		case "status":
			return nil, r.cause
		case "fresh-status":
			if count > 1 {
				return nil, r.cause
			}
		case "nil-status":
			return nil, nil
		}
	}
	return r.Repository.ReadChanges(ctx, dir, branch)
}
func (r *failingInspection) IgnoredFiles(ctx context.Context, dir string) ([]string, error) {
	if dir == r.path && r.mode == "ignored" {
		return nil, r.cause
	}
	return r.Repository.IgnoredFiles(ctx, dir)
}
func (r *failingInspection) WorktreeRemove(ctx context.Context, root, path string, force bool) error {
	r.removes.Add(1)
	return r.Repository.WorktreeRemove(ctx, root, path, force)
}

func TestRemovalFailsClosedOnIncompleteInspectionEvenWithForce(t *testing.T) {
	for _, mode := range []string{"status", "fresh-status", "nil-status", "ignored"} {
		for _, force := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/normal", true: "/force"}[force], func(t *testing.T) {
				main, secondary := setupTestGitRepo(t)
				native, err := git.NewNativeRunner()
				if err != nil {
					t.Fatal(err)
				}
				secret := filepath.Join(secondary, ".env")
				if err := os.WriteFile(secret, []byte("preserve secret"), 0600); err != nil {
					t.Fatal(err)
				}
				cause := errors.New("inspection unavailable")
				repo := &failingInspection{Repository: git.NewRepository(native), path: secondary, mode: mode, cause: cause}
				_, err = app.NewApp(repo).RemoveWorkspace(context.Background(), main, "feat/test", force)
				if err == nil || repo.removes.Load() != 0 {
					t.Fatalf("incomplete audit reached removal: %v %d", err, repo.removes.Load())
				}
				if mode != "nil-status" && !errors.Is(err, cause) {
					t.Fatalf("inspection cause lost: %v", err)
				}
				if data, err := os.ReadFile(secret); err != nil || string(data) != "preserve secret" {
					t.Fatal("failed inspection lost user data")
				}
			})
		}
	}
}

type residualRepository struct {
	app.Repository
	symlinkTarget string
}

func (r *residualRepository) WorktreeRemove(ctx context.Context, root, path string, force bool) error {
	if err := r.Repository.WorktreeRemove(ctx, root, path, force); err != nil {
		return err
	}
	if r.symlinkTarget != "" {
		return os.Symlink(r.symlinkTarget, path)
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, ".env"), []byte("new owner data"), 0600)
}
func TestRemovalPreservesReusedPathAndSymlinkAfterNativeSuccess(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "directory", true: "symlink"}[symlink], func(t *testing.T) {
			main, secondary := setupTestGitRepo(t)
			native, err := git.NewNativeRunner()
			if err != nil {
				t.Fatal(err)
			}
			repo := &residualRepository{Repository: git.NewRepository(native)}
			if symlink {
				repo.symlinkTarget = filepath.Join(t.TempDir(), "missing-destination")
			}
			_, err = app.NewApp(repo).RemoveWorkspace(context.Background(), main, "feat/test", true)
			if err == nil || !strings.Contains(err.Error(), "residual data preserved") {
				t.Fatalf("residue not reported: %v", err)
			}
			if _, err := os.Lstat(secondary); err != nil {
				t.Fatal("replacement path was deleted")
			}
			if !symlink {
				if data, err := os.ReadFile(filepath.Join(secondary, ".env")); err != nil || string(data) != "new owner data" {
					t.Fatal("residual data removed")
				}
			}
			worktrees, err := git.NewRepository(native).ReadWorktrees(context.Background(), main)
			if err != nil {
				t.Fatal(err)
			}
			for _, ws := range worktrees {
				if ws.Path == secondary {
					t.Fatal("native removal did not remove registry")
				}
			}
		})
	}
}

func TestSafetyRefreshesCleanSnapshotAndProtectsNestedActiveWorkspace(t *testing.T) {
	main, secondary := setupTestGitRepo(t)
	native, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	core := app.NewApp(git.NewRepository(native))
	ctx := context.Background()
	report, err := core.AuditWorkspaceSafety(ctx, main, &domain.Workspace{Path: secondary, Status: &domain.ChangeSet{}})
	if err != nil || report.IsSafe {
		t.Fatalf("stale clean status hid untracked data: %+v %v", report, err)
	}
	nested := filepath.Join(secondary, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	_, err = core.RemoveWorkspace(ctx, nested, "feat/test", true)
	if err == nil || !strings.Contains(err.Error(), "active workspace") {
		t.Fatalf("nested active workspace not protected: %v", err)
	}
	if _, err := os.Stat(secondary); err != nil {
		t.Fatal("active workspace was removed")
	}
}
