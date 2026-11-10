package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// VORQ-011 regression tests: the Studio never re-reads on a timer, and reads
// that do happen keep the user's selection, scroll and expanded folders.

func manyChanges(t *testing.T, m *Model, dir string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		writeStudio(t, dir, fmt.Sprintf("many/f%02d.txt", i), "x\n")
	}
	drive(t, m, m.refresh())
}

func wheel(m *Model, x, y int, button tea.MouseButton) {
	m.mouse(tea.MouseMsg{X: x, Y: y, Button: button, Action: tea.MouseActionPress})
}

func TestTickNeverRefreshesWorkspace(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	loadID, fileID := m.loadID, m.fileID
	for i := 0; i < 100; i++ {
		m.Update(tickMsg(time.Now().Add(time.Duration(i) * time.Minute)))
	}
	if m.loadID != loadID || m.fileID != fileID || m.loading {
		t.Fatalf("tick started a read: load %d→%d file %d→%d", loadID, m.loadID, fileID, m.fileID)
	}
}

func TestRefreshKeepsWheelScrollAndSelection(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	manyChanges(t, m, dir, 60)
	selectPath(t, m, "many/f01.txt")
	selected := m.selected
	for i := 0; i < 5; i++ {
		wheel(m, 2, m.bodyTop()+5, tea.MouseButtonWheelDown)
	}
	offset := m.treeOffset
	if offset == 0 || m.selected != selected {
		t.Fatalf("wheel must scroll the list only: offset=%d selected %d→%d", offset, selected, m.selected)
	}
	drive(t, m, m.refresh())
	if m.treeOffset != offset {
		t.Fatalf("refresh moved the list: offset %d→%d", offset, m.treeOffset)
	}
	if node := m.selectedNode(); node == nil || node.Path != "many/f01.txt" {
		t.Fatalf("refresh changed selection to %v", node)
	}
}

func TestRefreshFollowsSelectionByPathWhenRowsShift(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	manyChanges(t, m, dir, 10)
	selectPath(t, m, "many/f05.txt")
	// New files sorted before the selection shift every row index.
	writeStudio(t, dir, "a-first.txt", "x\n")
	writeStudio(t, dir, "a-second.txt", "x\n")
	drive(t, m, m.refresh())
	if node := m.selectedNode(); node == nil || node.Path != "many/f05.txt" {
		t.Fatalf("selection jumped to %v", node)
	}
}

func TestNavigationWorksDuringRead(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	manyChanges(t, m, dir, 5)
	selectPath(t, m, "many/f01.txt")
	m.loading = true
	press(m, "down")
	if node := m.selectedNode(); node == nil || node.Path != "many/f02.txt" {
		t.Fatalf("down ignored while loading: %v", node)
	}
}

func TestViewerWheelScrollsAnywhereOverPane(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	writeStudio(t, dir, "long.txt", strings.Repeat("line\n", 300))
	drive(t, m, m.refresh())
	selectPath(t, m, "long.txt")
	left, _, _ := m.paneWidths()
	// Wheel over the viewer tabs (first pane row) and over its footer.
	for _, y := range []int{m.bodyTop(), m.bodyTop() + m.panelHeight() - 1} {
		before := m.viewer.offset
		wheel(m, left+5, y, tea.MouseButtonWheelDown)
		if m.viewer.offset <= before {
			t.Fatalf("wheel at y=%d did not scroll viewer (%d→%d)", y, before, m.viewer.offset)
		}
	}
	// A refresh keeps the viewer position.
	offset := m.viewer.offset
	drive(t, m, m.refresh())
	if m.viewer.offset != offset {
		t.Fatalf("refresh reset viewer scroll %d→%d", offset, m.viewer.offset)
	}
}

func TestRefreshButtonIsClickableAtAllSizes(t *testing.T) {
	for _, size := range [][2]int{{160, 42}, {120, 30}, {80, 24}, {50, 14}} {
		m, _, _, _, _ := setupStudio(t)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		x, y := mouseLabelPosition(t, m.renderView(), refreshButton)
		loadID := m.loadID
		cmd := mousePress(m, x+1, y)
		if cmd == nil || m.loadID == loadID || !m.loading {
			t.Fatalf("size %v: refresh button did not start a read", size)
		}
		drive(t, m, cmd)
		assertFrame(t, m)
	}
}

func TestExpandedIgnoredFolderSurvivesRefresh(t *testing.T) {
	m, core, _, dir, _ := setupStudio(t)
	for i := 0; i < 30; i++ {
		writeStudio(t, dir, fmt.Sprintf("node_modules/pkg%02d/index.js", i), "x\n")
	}
	drive(t, m, m.refresh())
	drive(t, m, press(m, "e"))
	selectPath(t, m, "node_modules")
	drive(t, m, press(m, "enter"))
	selectPath(t, m, "node_modules/pkg07")
	reads := core.inventoryCalls
	drive(t, m, m.refresh()) // implicit refresh (after an action)
	if core.inventoryCalls != reads {
		t.Fatal("implicit refresh re-read an expanded ignored folder")
	}
	if node := m.selectedNode(); node == nil || node.Path != "node_modules/pkg07" {
		t.Fatalf("selection lost inside expanded folder: %v", node)
	}
	driveAll(t, m, m.reloadWorkspace()) // explicit Atualizar re-reads it once
	if core.inventoryCalls != reads+1 {
		t.Fatalf("explicit refresh should re-read expanded folder once, reads %d→%d", reads, core.inventoryCalls)
	}
	if node := m.selectedNode(); node == nil || node.Path != "node_modules/pkg07" {
		t.Fatalf("selection jumped after folder reload: %v", node)
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
}

func TestTreeRowsKeepPaneWidth(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	for _, name := range []string{"a.tsx", "b.jsx", "c.toml", "d.yaml", "e.png", ".env.local"} {
		writeStudio(t, dir, "icons/"+name, "x\n")
	}
	drive(t, m, m.refresh())
	left, _, _ := m.paneWidths()
	for _, line := range strings.Split(m.treeView(left), "\n") {
		if w := ansi.StringWidth(line); w != left {
			t.Fatalf("tree row width %d != %d: %q", w, left, ansi.Strip(line))
		}
		if strings.ContainsRune(line, '️') {
			t.Fatalf("tree row contains a variation selector: %q", ansi.Strip(line))
		}
	}
}

func (s *observedService) ReadInventoryDirectory(ctx context.Context, dir, path string, offset int) ([]*domain.TreeNode, error) {
	s.inventoryCalls++
	return s.Service.ReadInventoryDirectory(ctx, dir, path, offset)
}

// driveAll settles a command chain, executing batched commands as Bubble Tea
// would (sequentially here, which is deterministic for tests).
func driveAll(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 100 {
			t.Fatal("command chain did not settle")
		}
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		msg := next()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		_, follow := m.Update(msg)
		queue = append(queue, follow)
	}
}
