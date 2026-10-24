package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type appearanceStore struct {
	path     string
	mu       sync.Mutex
	revision uint64
}
type appearancePreference struct {
	Theme string `json:"theme"`
}
type appearanceMsg struct {
	revision uint64
	next     tea.Msg
	err      error
}

func userAppearanceStore() (*appearanceStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("diretório de configuração deve ser absoluto")
	}
	return &appearanceStore{path: filepath.Join(dir, "vorq", "appearance.json")}, nil
}

func (s *appearanceStore) load() (string, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var pref appearancePreference
	if err := json.Unmarshal(data, &pref); err != nil {
		return "", fmt.Errorf("preferência de aparência inválida: %w", err)
	}
	return pref.Theme, nil
}

func (s *appearanceStore) save(theme string, revision uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if revision <= s.revision && s.revision > 0 {
		return nil
	}
	data, err := json.MarshalIndent(appearancePreference{Theme: theme}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".appearance-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path); err != nil {
		return err
	}
	s.revision = revision
	return nil
}

func (m *Model) restoreAppearance(store *appearanceStore) {
	m.appearance = store
	name, err := store.load()
	if err != nil {
		m.notice = "Não foi possível ler o tema salvo: " + err.Error()
		return
	}
	if name == "" {
		return
	}
	for index, palette := range studioPalettes {
		if palette.name == name {
			m.themeIndex = index
			return
		}
	}
	m.notice = "Tema salvo indisponível; usando Tokyo Night."
}

func (m *Model) selectTheme(index int) tea.Cmd {
	m.themeIndex = index
	m.themeRevision++
	m.viewer.loading = true
	read := m.loadSelected()
	store, theme, revision := m.appearance, m.themeName(), m.themeRevision
	if store == nil {
		return read
	}
	return func() tea.Msg {
		err := store.save(theme, revision)
		var next tea.Msg
		if read != nil {
			next = read()
		}
		return appearanceMsg{revision: revision, next: next, err: err}
	}
}
