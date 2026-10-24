package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppearancePersistsSelectedThemeAcrossModels(t *testing.T) {
	store := &appearanceStore{path: filepath.Join(t.TempDir(), "vorq", "appearance.json")}
	m, _, _, _, _ := setupStudio(t)
	m.restoreAppearance(store)
	press(m, "t")
	drive(t, m, press(m, "3"))
	if m.themeName() != "Catppuccin" {
		t.Fatal("choice not applied")
	}
	reopened := NewModel(context.Background(), nil, "/another/worktree")
	reopened.restoreAppearance(&appearanceStore{path: store.path})
	if reopened.themeName() != "Catppuccin" {
		t.Fatal("theme lost across sessions/worktrees")
	}
	info, err := os.Stat(store.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("preference permissions: %v %v", info, err)
	}
	entries, err := os.ReadDir(filepath.Dir(store.path))
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary preference file left behind")
	}
}

func TestAppearanceSaveOrderAndInvalidConfig(t *testing.T) {
	store := &appearanceStore{path: filepath.Join(t.TempDir(), "appearance.json")}
	if err := store.save("Nord", 2); err != nil {
		t.Fatal(err)
	}
	if err := store.save("Gruvbox", 1); err != nil {
		t.Fatal(err)
	}
	if theme, err := store.load(); err != nil || theme != "Nord" {
		t.Fatal("old asynchronous save replaced newer choice")
	}
	for _, data := range []string{`{"theme":"removed theme"}`, `{invalid json`} {
		if err := os.WriteFile(store.path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		m := NewModel(context.Background(), nil, "/workspace")
		m.restoreAppearance(store)
		if m.themeName() != "Tokyo Night" || m.notice == "" {
			t.Fatal("invalid preference did not fall back visibly")
		}
	}
}

func TestAppearanceStoreUsesUserConfigAndSaveFailureIsVisible(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store, err := userAppearanceStore()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(store.path, filepath.Join("vorq", "appearance.json")) {
		t.Fatal("preference is not outside repository")
	}
	block := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(block, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	m, _, _, _, _ := setupStudio(t)
	m.appearance = &appearanceStore{path: filepath.Join(block, "appearance.json")}
	press(m, "t")
	drive(t, m, press(m, "4"))
	if m.themeName() != "Nord" || !strings.Contains(m.notice, "não foi possível salvar") {
		t.Fatal("save failure is silent or reverted session theme")
	}
	if data, err := os.ReadFile(block); err != nil || string(data) != "preserve" {
		t.Fatal("failed persistence damaged existing data")
	}
}
