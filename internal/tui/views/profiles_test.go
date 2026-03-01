package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"i9c/internal/aws"
)

func TestProfilesEnterLiveSelectsProfile(t *testing.T) {
	m := NewProfilesModel()
	m.SetRows([]ProfileRow{{Name: "dev", Status: string(aws.StatusLive)}})
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected select command")
	}
	msg := cmd()
	sel, ok := msg.(ProfileSelectMsg)
	if !ok || sel.Profile != "dev" {
		t.Fatalf("expected select dev, got %#v", msg)
	}
}

func TestProfilesEnterExpiredTriggersLogin(t *testing.T) {
	m := NewProfilesModel()
	m.SetRows([]ProfileRow{{Name: "dev", Status: string(aws.StatusExpired)}})
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected login command")
	}
	msg := cmd()
	login, ok := msg.(ProfileLoginMsg)
	if !ok || login.Profile != "dev" {
		t.Fatalf("expected login dev, got %#v", msg)
	}
}

func TestProfilesFilterAndView(t *testing.T) {
	m := NewProfilesModel()
	m.SetSize(120, 20)
	m.SetRows([]ProfileRow{
		{Name: "dev", Status: string(aws.StatusLive), Region: "us-east-1"},
		{Name: "prod", Status: string(aws.StatusNoSession), Region: "us-west-2"},
	})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	out := m.View()
	if !strings.Contains(out, "prod") {
		t.Fatalf("expected filtered row in view, got %s", out)
	}
}

func TestProfilesCancelLoginKey(t *testing.T) {
	m := NewProfilesModel()
	m.SetSize(120, 20)
	m.SetRows([]ProfileRow{{Name: "dev", Status: string(aws.StatusExpired)}})
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatal("expected cancel login command")
	}
	if _, ok := cmd().(ProfileCancelLoginMsg); !ok {
		t.Fatalf("expected ProfileCancelLoginMsg, got %#v", cmd())
	}
	if out := m.View(); !strings.Contains(out, "Status: Cancel login requested") {
		t.Fatalf("expected status indicator in view, got %s", out)
	}
}
