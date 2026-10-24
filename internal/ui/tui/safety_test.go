package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestSafety_MutationsRequireValidIdleSnapshot(t *testing.T) {
	for _, state := range []string{"stale", "loading", "busy", "mutating", "missing"} {
		t.Run(state, func(t *testing.T) {
			m, core, _, _, _ := setupStudio(t)
			m.commitSummary = "feat: proposed"
			switch state {
			case "stale":
				m.stale = true
			case "loading":
				m.loading = true
			case "busy":
				m.busy = "In progress"
			case "mutating":
				m.mutating = true
			case "missing":
				m.studio = nil
			}
			actions := map[string]func() tea.Cmd{"stage": m.stage, "review": m.review, "commit": m.commit, "push": m.push, "inspectPush": m.inspectPush, "inspectWorktrees": m.inspectWorktrees, "propose": func() tea.Cmd { return m.propose("feat") }, "switch": func() tea.Cmd { return m.switchWorkspace("/not-opened") }}
			for name, action := range actions {
				if cmd := action(); cmd != nil {
					t.Errorf("%s issued command while %s", name, state)
				}
			}
			for _, key := range []string{"s", "p", "w", "ctrl+s", "ctrl+g"} {
				if cmd := press(m, key); cmd != nil {
					t.Errorf("shortcut %s issued command while %s", key, state)
				}
			}
			if core.stageCalls != 0 || core.commitCalls != 0 || core.pushCalls != 0 {
				t.Fatal("blocked operation reached core")
			}
		})
	}
}

func TestSafety_RefreshFailureKeepsSnapshotAndBlocksUntilRecovery(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	snapshot := m.studio
	m.commitSummary = "feat: preserve draft"
	m.Update(treeMsg{id: m.loadID, err: errors.New("native status unavailable")})
	if !m.stale || m.studio != snapshot || m.commitSummary != "feat: preserve draft" {
		t.Fatal("failed refresh discarded valid snapshot/draft or lost stale marker")
	}
	if !strings.Contains(ansi.Strip(m.View()), "DESATUALIZADO") {
		t.Fatal("stale snapshot is not visibly marked")
	}
	if cmd := press(m, "s"); cmd != nil {
		t.Fatal("stale snapshot allowed mutation")
	}
	drive(t, m, press(m, "r"))
	if m.stale || m.loading || m.studio == nil {
		t.Fatal("valid retry did not clear stale state")
	}
}

func TestSafety_InitialLoadingHasNoInventedCleanOrZeroState(t *testing.T) {
	m := NewModel(context.Background(), nil, "/real/path/not-yet-inspected")
	text := ansi.Strip(m.View())
	for _, invented := range []string{"Repositório limpo", "0 arquivos alterados", "0 alterados", "↑0", "↓0", "Nenhum arquivo no stage"} {
		if strings.Contains(text, invented) {
			t.Errorf("unknown snapshot rendered as %q", invented)
		}
	}
	if !strings.Contains(text, "… arquivos") {
		t.Fatal("unknown counts lack placeholder")
	}
}

func TestSafety_PartialUnstageRequiresConfirmationAndKeepsWorkingBytes(t *testing.T) {
	m, core, r, dir, _ := setupStudio(t)
	ctx := context.Background()
	if err := r.Add(ctx, dir, "file.go"); err != nil {
		t.Fatal(err)
	}
	contents := "package main\n// staged version plus later working edit\n"
	writeStudio(t, dir, "file.go", contents)
	drive(t, m, m.refresh())
	selectPath(t, m, "file.go")
	if cmd := press(m, "s"); cmd != nil || m.modal != "unstage" || core.stageCalls != 0 {
		t.Fatal("partial file was unstaged before confirmation")
	}
	notice := ansi.Strip(m.modalView())
	if !strings.Contains(notice, "toda a preparação") || !strings.Contains(notice, "preservado") || !strings.Contains(notice, "file.go") {
		t.Fatal("partial unstage warning omits effect or selected path")
	}
	drive(t, m, press(m, "esc"))
	if core.stageCalls != 0 {
		t.Fatal("cancel modified index")
	}
	drive(t, m, press(m, "s"))
	drive(t, m, press(m, "enter"))
	if core.stageCalls != 1 {
		t.Fatalf("expected one confirmed unstage, got %d", core.stageCalls)
	}
	data, err := os.ReadFile(filepath.Join(dir, "file.go"))
	if err != nil || string(data) != contents {
		t.Fatalf("working copy changed: %q %v", data, err)
	}
	diff, err := r.DiffCached(ctx, dir)
	if err != nil || diff != "" {
		t.Fatalf("selected file still staged: %q %v", diff, err)
	}
}

func TestSafety_ReviewListsAllActualPreparedPathsAndCancelKeepsDraft(t *testing.T) {
	m, core, r, dir, _ := setupStudio(t)
	writeStudio(t, dir, "ação.txt", "real untracked data\n")
	if err := r.Add(context.Background(), dir, "file.go", "ação.txt"); err != nil {
		t.Fatal(err)
	}
	drive(t, m, m.refresh())
	m.commitSummary = "feat: real prepared files"
	m.commitDescription = "Body with actual intention"
	draft := m.draftMessage()
	drive(t, m, m.review())
	modal := ansi.Strip(m.modalView())
	for _, path := range []string{"file.go", "ação.txt"} {
		if !strings.Contains(modal, path) {
			t.Errorf("missing prepared path %s in review", path)
		}
	}
	if m.commitCount != 2 || m.reviewMessage != draft {
		t.Fatal("review omitted complete draft/count")
	}
	before, err := r.Status(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	drive(t, m, press(m, "esc"))
	after, err := r.Status(context.Background(), dir)
	if err != nil || before != after || m.draftMessage() != draft || core.commitCalls != 0 {
		t.Fatal("cancel changed stage, draft or created a commit")
	}
}

func TestSafety_WorktreeRestoresPathAndContextAgainstFreshInventory(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	ctx := context.Background()
	other := filepath.Join(t.TempDir(), "linked")
	if err := r.WorktreeAdd(ctx, dir, "other", other, "main", false); err != nil {
		t.Fatal(err)
	}
	m.switchTree("repositorio")
	selectPath(t, m, "file.go")
	m.commitSummary = "feat: original draft"
	m.commitDescription = "Original body"
	m.focusPane = "viewer"
	m.setViewerTab("arquivo")
	m.viewer.offset = 2
	m.viewer.column = 7
	drive(t, m, m.switchWorkspace(other))
	if m.commitSummary != "" {
		t.Fatal("draft leaked into another worktree")
	}
	m.commitSummary = "fix: other draft"
	// This changes list positions while the original worktree is not active.
	writeStudio(t, dir, "aaa-new.go", "package changed\n")
	drive(t, m, m.switchWorkspace(dir))
	selected := m.selectedNode()
	if selected == nil || selected.Path != "file.go" {
		t.Fatalf("selection restored by obsolete index instead of path: %+v", selected)
	}
	if m.commitSummary != "feat: original draft" || m.commitDescription != "Original body" || m.activeTab != "repositorio" || m.focusPane != "viewer" || m.viewerTab != "arquivo" || m.viewer.offset != 2 || m.viewer.column != 7 {
		t.Fatalf("lost workspace context: tab=%s focus=%s mode=%s offset=%d column=%d draft=%q", m.activeTab, m.focusPane, m.viewerTab, m.viewer.offset, m.viewer.column, m.commitSummary)
	}
	if m.changeSummary().Total != 2 {
		t.Fatal("restoring context reused obsolete index counts")
	}
	drive(t, m, m.switchWorkspace(other))
	if m.commitSummary != "fix: other draft" {
		t.Fatal("second worktree draft was overwritten")
	}
}

type partialCommitService struct {
	app.Service
	unknownHash bool
	calls       int
}

func (s *partialCommitService) CreateReviewedCommit(ctx context.Context, dir, message, diff string) (*domain.CommitResult, error) {
	s.calls++
	result, err := s.Service.CreateReviewedCommit(ctx, dir, message, diff)
	if err != nil {
		return result, err
	}
	outcome := &domain.CommitMetadataError{Hash: result.Hash, Operation: "reading files", Err: errors.New("metadata unavailable")}
	if s.unknownHash {
		outcome.Hash = ""
		return nil, outcome
	}
	return result, outcome
}
func TestSafety_PostCommitMetadataFailureNeverOffersCreationRetry(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		name := "known hash"
		if unknown {
			name = "unknown hash"
		}
		t.Run(name, func(t *testing.T) {
			m, core, r, dir, _ := setupStudio(t)
			if err := r.Add(context.Background(), dir, "file.go"); err != nil {
				t.Fatal(err)
			}
			drive(t, m, m.refresh())
			m.commitSummary = "feat: exactly once"
			service := &partialCommitService{Service: core, unknownHash: unknown}
			m.core = service
			drive(t, m, m.review())
			drive(t, m, press(m, "enter"))
			if m.commitSummary != "" || m.commitDescription != "" || service.calls != 1 {
				t.Fatal("created outcome kept reusable draft or repeated operation")
			}
			modal := ansi.Strip(m.modalView())
			if !strings.Contains(modal, "Commit criado") || strings.Contains(modal, "OPERAÇÃO NÃO CONCLUÍDA") {
				t.Fatalf("created commit presented as failed creation: %s", modal)
			}
			drive(t, m, press(m, "enter"))
			if service.calls != 1 {
				t.Fatal("metadata warning retried commit")
			}
			count, err := r.Exec(context.Background(), dir, "rev-list", "--count", "HEAD")
			if err != nil || strings.TrimSpace(count) != "2" {
				t.Fatalf("unexpected history %q %v", count, err)
			}
		})
	}
}

func TestSafety_FailedWorktreeInspectionPreservesCurrentWorkspace(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	snapshot := m.studio
	m.commitSummary = "feat: keep original"
	m.commitDescription = "Original description"
	drive(t, m, m.switchWorkspace(filepath.Join(t.TempDir(), "missing")))
	if m.dir != dir || m.studio != snapshot || m.commitSummary != "feat: keep original" || m.commitDescription != "Original description" || m.loading {
		t.Fatal("invalid target replaced current workspace or discarded draft")
	}
	if m.modal != "error" || !strings.Contains(m.lastError, "contexto anterior preservado") {
		t.Fatal("invalid worktree lacks contextual failure")
	}
}

func TestSafety_StaleWorkspaceReadCannotReplaceCurrentSnapshot(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	snapshot := m.studio
	oldID := m.loadID
	m.loadID++
	_, cmd := m.Update(workspaceMsg{id: oldID, tree: &domain.WorkspaceTree{Workspace: domain.Workspace{Path: "/obsolete"}}})
	if cmd != nil || m.dir != dir || m.studio != snapshot {
		t.Fatal("obsolete worktree response replaced active context")
	}
}

func safetyModalLabelCell(t *testing.T, m *Model, label string, last bool) (int, int) {
	t.Helper()
	box := m.modalView()
	lines := strings.Split(box, "\n")
	for n := range lines {
		y := n
		if last {
			y = len(lines) - 1 - n
		}
		line := ansi.Strip(lines[y])
		if index := strings.Index(line, label); index >= 0 {
			return max(0, (m.width-lipgloss.Width(box))/2) + ansi.StringWidth(line[:index]) + 1, max(0, (m.height-len(lines))/2) + y
		}
	}
	t.Fatalf("modal label %q missing", label)
	return 0, 0
}

func TestSafety_ModalBodyTextNeverActsAsConfirmationOrCancel(t *testing.T) {
	for _, label := range []string{"[Confirmar]", "[Voltar]"} {
		t.Run(label, func(t *testing.T) {
			m, core, r, dir, _ := setupStudio(t)
			m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
			if err := r.Add(context.Background(), dir, "file.go"); err != nil {
				t.Fatal(err)
			}
			drive(t, m, m.refresh())
			m.commitSummary = "feat: document literal " + label + " safely"
			drive(t, m, m.review())
			x, y := safetyModalLabelCell(t, m, label, false)
			if cmd := mousePress(m, x, y); cmd != nil || m.mutating || m.modal != "commit" || core.commitCalls != 0 {
				t.Fatalf("message-body literal %s activated a modal action", label)
			}
			// The actual footer action remains interactive after body text is ignored.
			x, y = safetyModalLabelCell(t, m, label, true)
			cmd := mousePress(m, x, y)
			if label == "[Confirmar]" {
				if cmd == nil || !m.mutating {
					t.Fatal("actual confirmation button did not schedule commit")
				}
				drive(t, m, cmd)
				if core.commitCalls != 1 || core.pushCalls != 0 {
					t.Fatal("actual footer confirmation did not create exactly one local commit")
				}
			} else {
				if cmd != nil || m.modal != "" || core.commitCalls != 0 || m.commitSummary == "" {
					t.Fatal("actual cancel button failed to preserve stage/draft")
				}
			}
		})
	}
}
