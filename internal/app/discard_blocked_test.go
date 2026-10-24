package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscardBlocksRenameConflictAndGitlink(t *testing.T) {
	for _, kind := range []string{"rename", "conflict", "submodule", "submodule-deleted", "symlink-deleted"} {
		t.Run(kind, func(t *testing.T) {
			a, r, dir := setupCommitApp(t)
			ctx := context.Background()
			writeCommitFile(t, dir, "old", "original\n")
			if err := r.Add(ctx, dir, "."); err != nil {
				t.Fatal(err)
			}
			if _, err := r.Commit(ctx, dir, "initial"); err != nil {
				t.Fatal(err)
			}
			path := "old"
			switch kind {
			case "rename":
				if _, err := r.Exec(ctx, dir, "mv", "--", "old", "new"); err != nil {
					t.Fatal(err)
				}
				path = "new"
			case "submodule", "submodule-deleted":
				head, err := r.RevParseHead(ctx, dir)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := r.Exec(ctx, dir, "update-index", "--add", "--cacheinfo", "160000,"+head+",module"); err != nil {
					t.Fatal(err)
				}
				path = "module"
				if kind == "submodule-deleted" {
					if _, err := r.Commit(ctx, dir, "add gitlink"); err != nil {
						t.Fatal(err)
					}
					if _, err := r.Exec(ctx, dir, "rm", "--cached", "--", "module"); err != nil {
						t.Fatal(err)
					}
				}
			case "symlink-deleted":
				path = "link"
				if err := os.Symlink("old", filepath.Join(dir, path)); err != nil {
					t.Fatal(err)
				}
				if err := r.Add(ctx, dir, path); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Commit(ctx, dir, "add symlink"); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(dir, path)); err != nil {
					t.Fatal(err)
				}
			case "conflict":
				if _, err := r.Exec(ctx, dir, "checkout", "-b", "other"); err != nil {
					t.Fatal(err)
				}
				writeCommitFile(t, dir, "old", "other\n")
				if err := r.Add(ctx, dir, "."); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Commit(ctx, dir, "other"); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Exec(ctx, dir, "checkout", "main"); err != nil {
					t.Fatal(err)
				}
				writeCommitFile(t, dir, "old", "main\n")
				if err := r.Add(ctx, dir, "."); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Commit(ctx, dir, "main"); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Exec(ctx, dir, "merge", "other"); err == nil {
					t.Fatal("fixture did not conflict")
				}
			}
			before, err := r.Status(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.PrepareDiscard(ctx, dir, path); err == nil {
				t.Fatalf("%s allowed", kind)
			}
			after, err := r.Status(ctx, dir)
			if err != nil || after != before {
				t.Fatal("blocked discard mutated status")
			}
		})
	}
}
