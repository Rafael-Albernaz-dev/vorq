package suggest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOfficialModelCatalogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	data := `{"models":[{"slug":"gpt-test","display_name":"Test model","visibility":"list"},{"slug":"gpt-hidden","visibility":"hide"},{"slug":"other-vendor","visibility":"list"}]}`
	if err := os.WriteFile(filepath.Join(home, ".codex", "models_cache.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	models, err := NewCLIProvider().ListModels(context.Background(), "codex")
	if err != nil || len(models) != 2 || models[1].ID != "gpt-test" {
		t.Fatalf("wrong catalog: %+v %v", models, err)
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	script := "#!/bin/sh\nprintf '%s\\n' 'Fetching available models...' 'gemini-test Gemini Test' 'claude-test Claude Test'\n"
	if err = os.WriteFile(filepath.Join(dir, "agy"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	models, err = NewCLIProvider().ListModels(context.Background(), "gemini")
	if err != nil || len(models) != 1 || models[0].ID != "gemini-test" {
		t.Fatalf("non-Gemini catalog leaked: %+v %v", models, err)
	}
}
