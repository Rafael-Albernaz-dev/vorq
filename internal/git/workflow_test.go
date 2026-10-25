package git_test

import (
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
	"testing"
)

func TestWebRepositoryURLRemovesCredentialsAndRejectsUnsafeData(t *testing.T) {
	for raw, want := range map[string]string{"git@github.com:owner/repo.git": "https://github.com/owner/repo", "https://user:secret@github.com/owner/repo.git?token=secret#fragment": "https://github.com/owner/repo", "ssh://git@github.com/owner/repo.git": "https://github.com/owner/repo", "/tmp/repo": "", "file:///tmp/repo": "", "https://github.com/owner/repo/extra": "", "https://github.com/owner/%1b": "", "https://github.com/../repo": ""} {
		got, _ := git.WebRepositoryURL(raw)
		if got != want {
			t.Fatalf("%q: got %q want %q", raw, got, want)
		}
	}
}
