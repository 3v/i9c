package views

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ilog "i9c/internal/logs"
	"i9c/internal/tui/theme"
)

type LogsModel struct {
	hub           *ilog.Hub
	width, height int
	channels      []string
	channelIndex  int
	scroll        int
}

func NewLogsModel(hub *ilog.Hub) *LogsModel {
	return &LogsModel{hub: hub, channels: []string{ilog.ChannelSystem, ilog.ChannelApp, ilog.ChannelDrift, ilog.ChannelAgent}}
}

func (m *LogsModel) SetSize(w, h int) { m.width, m.height = w, h }

func (m *LogsModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "h", "left":
			if m.channelIndex > 0 {
				m.channelIndex--
			}
		case "l", "right":
			if m.channelIndex < len(m.channels)-1 {
				m.channelIndex++
			}
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

func (m *LogsModel) View() string {
	ch := m.channels[m.channelIndex]
	lines := m.hub.Snapshot(ch)

	visible := m.height - 7
	if visible < 1 {
		visible = 1
	}
	if m.scroll > len(lines)-visible {
		m.scroll = max(0, len(lines)-visible)
	}
	start := m.scroll
	end := start + visible
	if end > len(lines) {
		end = len(lines)
	}

	var tabs []string
	for i, c := range m.channels {
		if i == m.channelIndex {
			tabs = append(tabs, theme.TabActiveStyle.Render(c))
		} else {
			tabs = append(tabs, theme.TabInactiveStyle.Render(c))
		}
	}

	body := "No logs yet."
	if len(lines) > 0 {
		body = strings.Join(lines[start:end], "\n")
	}
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Padding(1, 2).Render(
		theme.TitleStyle.Render("Logs")+"  "+strings.Join(tabs, " ")+"\n\n"+body+
			"\n\n[h/l] channel  [j/k] scroll",
	)
}
