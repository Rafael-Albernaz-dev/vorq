package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func TestDiscardConfirmationAndFileStates(t *testing.T) {
	for _, state := range []string{"untracked", "working", "staged", "both", "added", "deleted", "staged-delete", "unborn-added"} {
		t.Run(state, func(t *testing.T) {
			a, r, dir := setupCommitApp(t)
			ctx := context.Background()
			path := "ação [literal].go"
			if state != "unborn-added" {
				writeCommitFile(t, dir, path, "original\n")
				if err := r.Add(ctx, dir, "."); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Commit(ctx, dir, "initial"); err != nil {
					t.Fatal(err)
				}
			}
			if state == "untracked" || state == "added" || state == "unborn-added" {
				path = "novo [literal].go"
			}
			writeCommitFile(t, dir, path, "changed\n")
			if state == "deleted" || state == "staged-delete" {
				if err := os.Remove(filepath.Join(dir, path)); err != nil {
					t.Fatal(err)
				}
			}
			if state == "staged" || state == "both" || state == "added" || state == "staged-delete" || state == "unborn-added" {
				if err := a.StageFiles(ctx, dir, []string{path}); err != nil {
					t.Fatal(err)
				}
			}
			if state == "both" {
				writeCommitFile(t, dir, path, "later working edit\n")
			}
			writeCommitFile(t, dir, "other.txt", "must survive\n")
			plan, err := a.PrepareDiscard(ctx, dir, path)
			if err != nil {
				t.Fatal(err)
			}
			before, err := r.Status(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			bytesBefore, readBefore := os.ReadFile(filepath.Join(dir, path))
			if err := a.DiscardFile(ctx, dir, *plan, false); err == nil {
				t.Fatal("discard without confirmation")
			}
			after, _ := r.Status(ctx, dir)
			bytesAfter, readAfter := os.ReadFile(filepath.Join(dir, path))
			if before != after || string(bytesBefore) != string(bytesAfter) || os.IsNotExist(readBefore) != os.IsNotExist(readAfter) {
				t.Fatal("unconfirmed discard mutated data")
			}
			if err := a.DiscardFile(ctx, dir, *plan, true); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, path))
			if plan.Delete {
				if !os.IsNotExist(err) {
					t.Fatalf("new file remains: %v", err)
				}
			} else if err != nil || string(data) != "original\n" {
				t.Fatalf("restore: %q %v", data, err)
			}
			if data, err := os.ReadFile(filepath.Join(dir, "other.txt")); err != nil || string(data) != "must survive\n" {
				t.Fatal("unrelated data affected")
			}
			status, err := r.Status(ctx, dir)
			if err != nil || strings.Contains(status, path) {
				t.Fatalf("discard left changes: %q %v", status, err)
			}
		})
	}
}

func TestDiscardRejectsChangedAndForgedPlans(t *testing.T) {
	for _, change := range []string{"working", "index", "head", "impact", "workspace"} {
		t.Run(change, func(t *testing.T) {
			a, r, dir := setupCommitApp(t)
			ctx := context.Background()
			writeCommitFile(t, dir, "f.go", "original\n")
			if err := r.Add(ctx, dir, "."); err != nil {
				t.Fatal(err)
			}
			if _, err := r.Commit(ctx, dir, "initial"); err != nil {
				t.Fatal(err)
			}
			writeCommitFile(t, dir, "f.go", "changed\n")
			plan, err := a.PrepareDiscard(ctx, dir, "f.go")
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "working":
				writeCommitFile(t, dir, "f.go", "new data\n")
			case "index":
				if err := r.Add(ctx, dir, "f.go"); err != nil {
					t.Fatal(err)
				}
			case "head":
				if _, err := r.Exec(ctx, dir, "commit", "--allow-empty", "-m", "head advanced"); err != nil {
					t.Fatal(err)
				}
			case "impact":
				plan.Impact = "wrong impact"
			case "workspace":
				plan.WorkspacePath = t.TempDir()
			}
			before, _ := r.Status(ctx, dir)
			dataBefore, _ := os.ReadFile(filepath.Join(dir, "f.go"))
			if err := a.DiscardFile(ctx, dir, *plan, true); err == nil {
				t.Fatal("stale/forged plan accepted")
			}
			after, _ := r.Status(ctx, dir)
			dataAfter, _ := os.ReadFile(filepath.Join(dir, "f.go"))
			if before != after || string(dataBefore) != string(dataAfter) {
				t.Fatal("rejected plan mutated data")
			}
		})
	}
}

func TestDiscardProtectsIgnoredPathsAndMetadata(t *testing.T) {
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, ".gitignore", ".env\nignored/\n")
	writeCommitFile(t, dir, ".env", "secret\n")
	writeCommitFile(t, dir, "ignored/secret", "secret\n")
	writeCommitFile(t, dir, "plain.txt", "original\n")
	if err := r.Add(ctx, dir, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, dir, "initial"); err != nil {
		t.Fatal(err)
	}
	// Also protect a force-added ignored file.
	if _, err := r.Exec(ctx, dir, "add", "-f", "--", ".env"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "external")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".env", "ignored/secret", ".", ".git/config", "../outside", "external/file", "plain.txt"} {
		if _, err := a.PrepareDiscard(ctx, dir, path); err == nil {
			t.Errorf("protected/unchanged path allowed: %q", path)
		}
	}
	if err := a.DiscardFile(ctx, dir, domain.DiscardPlan{Path: ".env"}, true); err == nil {
		t.Fatal("forged ignored plan accepted")
	}
	if data, err := os.ReadFile(filepath.Join(dir, ".env")); err != nil || string(data) != "secret\n" {
		t.Fatal("secret changed")
	}
}

func TestDiscardLiteralNames(t *testing.T) {
	for _, name := range []string{"-f", ":(glob)*", "ação\n[Confirmar].txt"} {
		t.Run(name, func(t *testing.T) {
			a, _, dir := setupCommitApp(t)
			ctx := context.Background()
			writeCommitFile(t, dir, name, "delete only me\n")
			writeCommitFile(t, dir, "other", "keep\n")
			plan, err := a.PrepareDiscard(ctx, dir, name)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.DiscardFile(ctx, dir, *plan, true); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "other")); err != nil {
				t.Fatal("literal path expanded")
			}
		})
	}
}

func TestUnstageAllUnbornPreservesWorkingBytes(t *testing.T) {
	a, _, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "new", "staged\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "new", "later edit\n")
	if err := a.UnstageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	cs, err := a.GetCommitChanges(ctx, dir)
	if err != nil || cs.StagedCount() != 0 || cs.UntrackedCount() != 1 {
		t.Fatalf("unstage: %+v %v", cs, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "new")); err != nil || string(data) != "later edit\n" {
		t.Fatal("unstage changed working bytes")
	}
}
