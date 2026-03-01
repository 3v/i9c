package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"i9c/internal/config"
	ilog "i9c/internal/logs"
)

func TestAdvisorHotkeyDoesNotLeakIntoComposer(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IACDir = "."
	m := NewModel(cfg, ilog.NewHub(100))

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if m.activeTab != TabAdvisor {
		t.Fatalf("expected active tab advisor, got %v", m.activeTab)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if got := m.advisor.ComposerValue(); got != "" {
		t.Fatalf("expected empty composer after duplicate panel hotkey, got %q", got)
	}
}

func TestAdvisorFirstTypedCharAfterSwitchStillWorks(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IACDir = "."
	m := NewModel(cfg, ilog.NewHub(100))

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	if got := m.advisor.ComposerValue(); got != "h" {
		t.Fatalf("expected first typed char to reach composer, got %q", got)
	}
}
