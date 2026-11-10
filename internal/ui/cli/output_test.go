package cli

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func cliRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "T"}, {"config", "user.email", "t@example.com"}, {"commit", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	return dir
}

func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// VORQ-012 (B7): runtime errors are reported without dumping usage.
func TestRuntimeErrorDoesNotPrintUsage(t *testing.T) {
	t.Chdir(t.TempDir()) // not a repository
	out, err := runRoot(t, "status")
	if err == nil {
		t.Fatal("status outside a repository must fail")
	}
	if strings.Contains(out, "Usage:") {
		t.Fatalf("usage printed on runtime error:\n%s", out)
	}
}

// VORQ-012 (B6): the list header is one line and columns align by visible width.
func TestListHeaderIsOneAlignedLine(t *testing.T) {
	t.Chdir(cliRepo(t))
	out, err := runRoot(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	var header, row string
	for _, line := range strings.Split(ansi.Strip(out), "\n") {
		if strings.Contains(line, "BRANCH") {
			header = line
		}
		if strings.Contains(line, "main") {
			row = line
		}
	}
	if header == "" || !strings.Contains(header, "PATH") || !strings.Contains(header, "STATUS") {
		t.Fatalf("header split across lines:\n%s", ansi.Strip(out))
	}
	colOf := func(line, word string) int { return ansi.StringWidth(line[:strings.Index(line, word)]) }
	if colOf(header, "PATH") != colOf(row, "./") {
		t.Fatalf("PATH column misaligned:\n%s\n%s", header, row)
	}
}
