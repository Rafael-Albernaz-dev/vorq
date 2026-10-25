package clipboard

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeClipboardExactTextAndFailure(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "copied")
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("DISPLAY", "")
	t.Setenv("COPY_OUTPUT", out)
	binary := filepath.Join(dir, "wl-copy")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n/bin/cat > \"$COPY_OUTPUT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	value := "/repo/nome ação [x] $(literal).go"
	if err := Copy(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if string(data) != value {
		t.Fatalf("altered path %q", data)
	}
	os.WriteFile(binary, []byte("#!/bin/sh\nexit 1\n"), 0700)
	if Copy(context.Background(), value) == nil {
		t.Fatal("false success")
	}
	os.Remove(binary)
	if Copy(context.Background(), value) == nil {
		t.Fatal("missing backend reported success")
	}
}
