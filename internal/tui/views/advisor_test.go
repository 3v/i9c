package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"i9c/internal/config"
)

func TestAdvisorShowsFocusHintWhenUnlockedAndBlurred(t *testing.T) {
	m := NewAdvisorModel(config.DefaultConfig())
	m.SetSize(100, 30)
	_ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	out := m.View()
	if !strings.Contains(out, "Press [i] to focus") {
		t.Fatalf("expected focus hint, got: %s", out)
	}
}

func TestAdvisorBusyViewShowsCancelHint(t *testing.T) {
	m := NewAdvisorModel(config.DefaultConfig())
	m.SetSize(100, 30)
	_ = m.Update(AdvisorBusyMsg{Busy: true})
	out := m.View()
	if !strings.Contains(out, "Press [x] to cancel") {
		t.Fatalf("expected cancel hint while busy, got: %s", out)
	}
}

func TestAdvisorActivityLineRenders(t *testing.T) {
	m := NewAdvisorModel(config.DefaultConfig())
	m.SetSize(100, 30)
	_ = m.Update(AdvisorActivityMsg{Text: "item started: reasoning", Running: true})
	out := m.View()
	if !strings.Contains(out, "Activity: item started: reasoning") {
		t.Fatalf("expected activity line, got: %s", out)
	}
}
