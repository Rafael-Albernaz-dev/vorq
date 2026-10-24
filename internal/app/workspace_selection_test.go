package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type selectionRepository struct {
	app.Repository
	workspaces []*domain.Workspace
}

func (r *selectionRepository) ProjectPaths(context.Context, string) (string, string, error) {
	return "/active", "/active/.git", nil
}
func (r *selectionRepository) ReadWorktrees(context.Context, string) ([]*domain.Workspace, error) {
	return r.workspaces, nil
}
func (r *selectionRepository) ReadChanges(context.Context, string, string) (*domain.ChangeSet, error) {
	return &domain.ChangeSet{}, nil
}
func (r *selectionRepository) AheadBehind(context.Context, string) (int, int, error) {
	return 0, 0, nil
}
func (r *selectionRepository) Diff(context.Context, string) (string, error)       { return "", nil }
func (r *selectionRepository) DiffCached(context.Context, string) (string, error) { return "", nil }

func TestWorkspaceSelectionRequiresUnambiguousTarget(t *testing.T) {
	for _, tc := range []struct {
		name, target, expectedPath, expectedError string
	}{
		{"exact path", "/one/task", "/one/task", ""},
		{"branch beats suffix", "task", "/branch", ""},
		{"ambiguous suffix", "same", "", "ambiguous"},
		{"missing active", "", "", "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &selectionRepository{workspaces: []*domain.Workspace{
				{Path: "/one/task", Branch: "feature"},
				{Path: "/branch", Branch: "task"},
				{Path: "/one/same", Branch: "first"},
				{Path: "/two/same", Branch: "second"},
			}}
			core := app.NewApp(repo)
			if tc.expectedError != "" {
				// Removal must stop before invoking any mutation on the repository.
				_, err := core.RemoveWorkspace(context.Background(), "/active", tc.target, true)
				if err == nil || !strings.Contains(err.Error(), tc.expectedError) {
					t.Fatalf("unsafe selection: %v", err)
				}
				return
			}
			ws, _, err := core.GetWorkspaceStatus(context.Background(), "/active", tc.target)
			if err != nil || ws == nil || ws.Path != tc.expectedPath {
				t.Fatalf("wrong selection: %+v %v", ws, err)
			}
		})
	}
}
