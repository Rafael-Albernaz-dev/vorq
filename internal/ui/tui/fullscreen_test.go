package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func fullscreenFixture(t *testing.T, w, h int) *Model {
	t.Helper()
	m, _, _, dir, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	long := strings.Repeat("x", 400)
	writeStudio(t, dir, "long.txt", strings.Repeat("line "+long+"\n", 200))
	drive(t, m, m.refresh())
	selectPath(t, m, "long.txt")
	return m
}

func clickLabel(t *testing.T, m *Model, label string) tea.Cmd {
	t.Helper()
	x, y := mouseLabelPosition(t, m.renderView(), label)
	return mousePress(m, x+1, y)
}

func TestFullscreenButtonEntersAndExitsWithoutReads(t *testing.T) {
	m := fullscreenFixture(t, 160, 42)
	m.viewer.offset, m.viewer.column = 7, 8
	loadID, fileID := m.loadID, m.fileID
	if cmd := clickLabel(t, m, fullscreenOff); cmd != nil || !m.fullscreen {
		t.Fatal(`"Tela cheia [ ]" must enter full screen without commands`)
	}
	if m.viewer.offset != 7 || m.viewer.column != 8 {
		t.Fatal("entering full screen moved the viewer")
	}
	press(m, "down")
	if m.viewer.offset != 8 {
		t.Fatalf("down did not scroll in full screen: %d", m.viewer.offset)
	}
	clickLabel(t, m, exitButton)
	if m.fullscreen || m.viewer.offset != 8 {
		t.Fatalf(`"Tela cheia [x]" must exit keeping scroll: fullscreen=%v offset=%d`, m.fullscreen, m.viewer.offset)
	}
	for _, key := range []string{"f", "esc", "f", "q", "f", "f"} {
		press(m, key)
	}
	if m.fullscreen {
		t.Fatal("f/Esc/q must toggle full screen")
	}
	if node := m.selectedNode(); node == nil || node.Path != "long.txt" {
		t.Fatal("selection changed")
	}
	if m.loadID != loadID || m.fileID != fileID {
		t.Fatal("full screen triggered a read")
	}
}

func TestFullscreenFrameFillsTerminalAtAllSizes(t *testing.T) {
	for _, size := range [][2]int{{160, 42}, {120, 30}, {80, 24}, {50, 14}} {
		m := fullscreenFixture(t, size[0], size[1])
		press(m, "f")
		if !m.fullscreen {
			t.Fatalf("size %v: not in full screen", size)
		}
		assertFrame(t, m)
		press(m, "z")
		assertFrame(t, m)
	}
}

func TestFullscreenWrapAndHorizontalScroll(t *testing.T) {
	m := fullscreenFixture(t, 120, 30)
	press(m, "f")
	press(m, "right")
	if m.viewer.column != 4 {
		t.Fatal("right must scroll horizontally without wrap")
	}
	clickLabel(t, m, wrapButton)
	if !m.viewerWrap || m.viewer.column != 0 {
		t.Fatal("[z Quebrar linhas] must enable wrap and reset the column")
	}
	view := ansi.Strip(m.renderView())
	if strings.Count(view, "xxxxxxxxxx") < 10 {
		t.Fatal("wrapped view should show continuation rows of the long line")
	}
	press(m, "right")
	if m.viewer.column != 0 {
		t.Fatal("horizontal scroll must be off while wrapping")
	}
	press(m, "end")
	if m.viewer.offset == 0 {
		t.Fatal("end must reach the last source lines while wrapping")
	}
}

func TestFullscreenWheelTabsAndAllowedActions(t *testing.T) {
	m := fullscreenFixture(t, 160, 42)
	press(m, "f")
	m.mouse(tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.viewer.offset != 3 {
		t.Fatalf("wheel did not scroll: %d", m.viewer.offset)
	}
	widths := tabWidths(viewerTabLabels, m.width)
	mousePress(m, widths[0]+widths[1]+1, 2)
	if m.viewerTab != "arquivo" {
		t.Fatalf("tab click ignored: %s", m.viewerTab)
	}
	// s stages the shown file (core call), x opens the discard confirmation.
	if cmd := press(m, "s"); cmd == nil {
		t.Fatal("s must stage in full screen")
	}
	m.busy, m.mutating = "", false
	if cmd := press(m, "x"); cmd == nil || m.modal != "discard" {
		t.Fatal("x must open discard confirmation in full screen")
	}
	assertFrame(t, m) // modal overlays the full-screen frame
	press(m, "esc")
	// Other global actions stay unavailable until leaving.
	for _, key := range []string{"p", "c", "w", "b"} {
		if cmd := press(m, key); cmd != nil || m.modal != "" || !m.fullscreen {
			t.Fatalf("%s must not act inside full screen", key)
		}
	}
}

func TestFullscreenEndsWhenFileLeavesTheList(t *testing.T) {
	m := fullscreenFixture(t, 160, 42)
	press(m, "f")
	if err := os.Remove(filepath.Join(m.dir, "long.txt")); err != nil {
		t.Fatal(err)
	}
	drive(t, m, m.refresh())
	if m.fullscreen || !strings.Contains(m.notice, "tela cheia encerrada") {
		t.Fatalf("full screen must end when its file disappears (fullscreen=%v notice=%q)", m.fullscreen, m.notice)
	}
	assertFrame(t, m)
}

// The toggle sits at the right edge of the summary row in every layout.
func TestFullscreenToggleIsAtSummaryRowRightEdge(t *testing.T) {
	for _, size := range [][2]int{{160, 42}, {120, 30}, {80, 24}, {50, 14}} {
		m := fullscreenFixture(t, size[0], size[1])
		row := ansi.Strip(strings.Split(m.renderView(), "\n")[m.summaryRowY()])
		if !strings.HasSuffix(strings.TrimRight(row, " "), fullscreenOff) {
			t.Fatalf("size %v: toggle not at the right of the summary row: %q", size, row)
		}
		mousePress(m, m.width-3, m.summaryRowY())
		if !m.fullscreen {
			t.Fatalf("size %v: clicking %q did not enter full screen", size, fullscreenOff)
		}
		title := ansi.Strip(strings.Split(m.renderView(), "\n")[0])
		if !strings.Contains(title, fullscreenOn) {
			t.Fatalf("size %v: full screen title lacks %q: %q", size, fullscreenOn, title)
		}
		assertFrame(t, m)
	}
}
