package git_test

import (
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func TestParseStatus(t *testing.T) {
	// Simulated NUL-delimited status output
	// M  staged_file.go
	//  M modified_unstaged.go
	// MM staged_and_modified.go
	// ?? untracked_file.txt
	// R  old_name.go -> new_name.go
	raw := "M  staged_file.go\x00" +
		" M modified_unstaged.go\x00" +
		"MM staged_and_modified.go\x00" +
		"?? untracked_file.txt\x00" +
		"R  new_name.go\x00old_name.go\x00"

	cs, err := git.ParseStatus(raw, "/path/to/ws", "main")
	if err != nil {
		t.Fatalf("unexpected error parsing status: %v", err)
	}

	if len(cs.Files) != 5 {
		t.Fatalf("expected 5 files, got %d", len(cs.Files))
	}

	// 1: Staged file
	if cs.Files[0].Path != "staged_file.go" || cs.Files[0].Staged != "M" || cs.Files[0].Unstaged != " " {
		t.Errorf("expected staged file, got %+v", cs.Files[0])
	}

	// 2: Modified unstaged
	if cs.Files[1].Path != "modified_unstaged.go" || cs.Files[1].Staged != " " || cs.Files[1].Unstaged != "M" {
		t.Errorf("expected unstaged file, got %+v", cs.Files[1])
	}

	// 3: Staged and modified
	if cs.Files[2].Path != "staged_and_modified.go" || cs.Files[2].Staged != "M" || cs.Files[2].Unstaged != "M" {
		t.Errorf("expected MM file, got %+v", cs.Files[2])
	}

	// 4: Untracked
	if cs.Files[3].Path != "untracked_file.txt" || cs.Files[3].Staged != "?" || cs.Files[3].Unstaged != "?" {
		t.Errorf("expected untracked file, got %+v", cs.Files[3])
	}

	// 5: Renamed
	if cs.Files[4].Path != "new_name.go" || cs.Files[4].OrigPath != "old_name.go" || cs.Files[4].Staged != "R" {
		t.Errorf("expected rename, got %+v", cs.Files[4])
	}

	// Counts
	if cs.StagedCount() != 3 { // staged_file, staged_and_modified, rename
		t.Errorf("expected 3 staged files, got %d", cs.StagedCount())
	}
	if cs.UnstagedCount() != 2 { // modified_unstaged, staged_and_modified
		t.Errorf("expected 2 unstaged files, got %d", cs.UnstagedCount())
	}
	if cs.UntrackedCount() != 1 {
		t.Errorf("expected 1 untracked file, got %d", cs.UntrackedCount())
	}
}

func TestParseStatus_Empty(t *testing.T) {
	cs, err := git.ParseStatus("", "/path/to/ws", "main")
	if err != nil {
		t.Fatalf("unexpected error parsing empty status: %v", err)
	}
	if !cs.IsClean() {
		t.Errorf("expected empty status to be clean")
	}
}
