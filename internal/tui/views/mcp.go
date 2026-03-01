package views

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/tui/theme"
)

type MCPUpdateMsg struct {
	ConfiguredPrimary  string
	ConfiguredFallback string
	ActiveBackend      string
	CodexConnection    string
	Event              string
	Error              string
	At                 time.Time
}

type MCPModel struct {
	width, height      int
	configuredPrimary  string
	configuredFallback string
	activeBackend      string
	codexConnection    string
	events             []string
	scroll             int
}

func NewMCPModel() *MCPModel {
	return &MCPModel{
		configuredPrimary:  "managed aws mcp",
		configuredFallback: "local aws mcp",
		activeBackend:      "in-process aws sdk discovery",
		codexConnection:    "disconnected",
	}
}

func (m *MCPModel) SetSize(w, h int) { m.width, m.height = w, h }

func (m *MCPModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case MCPUpdateMsg:
		if msg.ConfiguredPrimary != "" {
			m.configuredPrimary = msg.ConfiguredPrimary
		}
		if msg.ConfiguredFallback != "" {
			m.configuredFallback = msg.ConfiguredFallback
		}
		if msg.ActiveBackend != "" {
			m.activeBackend = msg.ActiveBackend
		}
		if msg.CodexConnection != "" {
			m.codexConnection = msg.CodexConnection
		}
		if msg.Event != "" {
			ts := msg.At
			if ts.IsZero() {
				ts = time.Now()
			}
			m.events = append(m.events, ts.Format("15:04:05")+" "+msg.Event)
		}
		if msg.Error != "" {
			ts := msg.At
			if ts.IsZero() {
				ts = time.Now()
			}
			m.events = append(m.events, ts.Format("15:04:05")+" ERROR: "+msg.Error)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.scroll++
		case "k", "up":
			if m.scroll > 0 {
				m.scroll--
			}
		}
	}
	return nil
}

func (m *MCPModel) View() string {
	title := theme.TitleStyle.Render("MCP Status")
	info := []string{
		"Primary: " + m.configuredPrimary,
		"Fallback: " + m.configuredFallback,
		"Active: " + m.activeBackend,
		"Codex: " + m.codexConnection,
	}

	visible := m.height - 12
	if visible < 1 {
		visible = 1
	}
	if m.scroll > len(m.events)-visible {
		m.scroll = max(0, len(m.events)-visible)
	}
	start := m.scroll
	end := start + visible
	if end > len(m.events) {
		end = len(m.events)
	}
	events := "No MCP events yet."
	if len(m.events) > 0 {
		events = strings.Join(m.events[start:end], "\n")
	}

	return lipgloss.NewStyle().Width(m.width).Height(m.height).Padding(1, 2).Render(
		title + "\n\n" +
			strings.Join(info, "\n") +
			"\n\nEvents\n" +
			events +
			"\n\n[j/k] scroll",
	)
}
