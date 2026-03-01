package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/config"
	"i9c/internal/tui/theme"
)

type DriftAction string

const (
	DriftCreate  DriftAction = "create"
	DriftUpdate  DriftAction = "update"
	DriftDelete  DriftAction = "delete"
	DriftReplace DriftAction = "replace"
)

type DriftEntry struct {
	Profile  string
	Address  string
	Type     string
	Action   DriftAction
	Before   map[string]interface{}
	After    map[string]interface{}
}

type DriftModel struct {
	cfg           *config.Config
	width, height int
	entries       []DriftEntry
	filtered      []DriftEntry
	cursor        int
	filterText    string
	profileFilter map[string]bool
	detailMode    bool
}

func NewDriftModel(cfg *config.Config) *DriftModel {
	return &DriftModel{
		cfg: cfg,
	}
}

func (m *DriftModel) Init() tea.Cmd {
	return nil
}

func (m *DriftModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *DriftModel) SetFilter(text string) {
	m.filterText = text
	m.applyFilter()
}

type DriftUpdateMsg struct {
	Entries []DriftEntry
}

func (m *DriftModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case DriftUpdateMsg:
		m.entries = msg.Entries
		m.applyFilter()
		return nil
	case tea.KeyMsg:
		if m.detailMode {
			switch msg.String() {
			case "esc", "backspace":
				m.detailMode = false
			}
			return nil
		}
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if len(m.filtered) > 0 {
				m.detailMode = true
			}
		case "G":
			m.cursor = max(0, len(m.filtered)-1)
		case "g":
			m.cursor = 0
		}
	}
	return nil
}

func (m *DriftModel) View() string {
	if m.detailMode && len(m.filtered) > 0 {
		return m.renderDetail()
	}
	return m.renderTable()
}

func (m *DriftModel) renderTable() string {
	title := theme.TitleStyle.Render("Drift Detection")

	if len(m.filtered) == 0 {
		msg := "No drift detected"
		if len(m.entries) == 0 {
			msg = "No scan results yet. Waiting for terraform plan..."
		}
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Padding(1, 2).
			Render(title + "\n\n" + lipgloss.NewStyle().Foreground(theme.TextDim).Render(msg))
	}

	headerFmt := "%-14s %-40s %-24s %s"
	header := theme.TableHeaderStyle.Render(fmt.Sprintf(headerFmt, "PROFILE", "ADDRESS", "TYPE", "ACTION"))

	visibleRows := m.height - 6
	if visibleRows < 1 {
		visibleRows = 1
	}

	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}
	end := start + visibleRows
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	var rows []string
	for i := start; i < end; i++ {
		e := m.filtered[i]
		actionStr := m.formatAction(e.Action)

		line := fmt.Sprintf("%-14s %-40s %-24s %s", e.Profile, truncate(e.Address, 40), truncate(e.Type, 24), actionStr)

		if i == m.cursor {
			rows = append(rows, theme.TableSelectedStyle.Render(line))
		} else {
			rows = append(rows, theme.TableRowStyle.Render(line))
		}
	}

	counter := lipgloss.NewStyle().Foreground(theme.TextDim).Render(
		fmt.Sprintf("  %d/%d resources drifted", len(m.filtered), len(m.entries)))

	table := strings.Join(append([]string{header}, rows...), "\n")

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Padding(1, 2).
		Render(title + counter + "\n\n" + table)
}

func (m *DriftModel) renderDetail() string {
	e := m.filtered[m.cursor]
	title := theme.TitleStyle.Render("Drift Detail")
	back := lipgloss.NewStyle().Foreground(theme.TextDim).Render("  [esc] back")

	lines := []string{
		title + back,
		"",
		theme.HelpKeyStyle.Render("Address:  ") + theme.TableRowStyle.Render(e.Address),
		theme.HelpKeyStyle.Render("Type:     ") + theme.TableRowStyle.Render(e.Type),
		theme.HelpKeyStyle.Render("Profile:  ") + theme.TableRowStyle.Render(e.Profile),
		theme.HelpKeyStyle.Render("Action:   ") + m.formatAction(e.Action),
		"",
		theme.HelpKeyStyle.Render("Changes:"),
	}

	if e.Before != nil || e.After != nil {
		lines = append(lines, m.renderDiff(e.Before, e.After)...)
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.TextDim).Render("  No detailed diff available"))
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

func (m *DriftModel) renderDiff(before, after map[string]interface{}) []string {
	allKeys := make(map[string]bool)
	for k := range before {
		allKeys[k] = true
	}
	for k := range after {
		allKeys[k] = true
	}

	var lines []string
	for k := range allKeys {
		bv, bOk := before[k]
		av, aOk := after[k]
		if bOk && aOk {
			bStr := fmt.Sprintf("%v", bv)
			aStr := fmt.Sprintf("%v", av)
			if bStr != aStr {
				lines = append(lines,
					theme.DriftDestroyStyle.Render(fmt.Sprintf("  - %s: %s", k, bStr)),
					theme.DriftAddStyle.Render(fmt.Sprintf("  + %s: %s", k, aStr)),
				)
			}
		} else if bOk {
			lines = append(lines,
				theme.DriftDestroyStyle.Render(fmt.Sprintf("  - %s: %v", k, bv)),
			)
		} else {
			lines = append(lines,
				theme.DriftAddStyle.Render(fmt.Sprintf("  + %s: %v", k, av)),
			)
		}
	}
	return lines
}

func (m *DriftModel) formatAction(action DriftAction) string {
	switch action {
	case DriftCreate:
		return theme.DriftAddStyle.Render("+ create")
	case DriftUpdate:
		return theme.DriftChangeStyle.Render("~ update")
	case DriftDelete:
		return theme.DriftDestroyStyle.Render("- delete")
	case DriftReplace:
		return theme.DriftDestroyStyle.Render("-/+ replace")
	default:
		return string(action)
	}
}

func (m *DriftModel) SetProfileFilter(active map[string]bool) {
	m.profileFilter = active
	m.applyFilter()
}

func (m *DriftModel) ClearServiceFilter() {
	m.filterText = ""
	m.applyFilter()
}

func (m *DriftModel) applyFilter() {
	m.filtered = nil
	lower := strings.ToLower(m.filterText)
	for _, e := range m.entries {
		if m.profileFilter != nil && !m.profileFilter[e.Profile] {
			continue
		}
		if m.filterText != "" {
			if !strings.Contains(strings.ToLower(e.Address), lower) &&
				!strings.Contains(strings.ToLower(e.Type), lower) &&
				!strings.Contains(strings.ToLower(e.Profile), lower) {
				continue
			}
		}
		m.filtered = append(m.filtered, e)
	}
	if len(m.filtered) == 0 && m.filterText == "" && m.profileFilter == nil {
		m.filtered = m.entries
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
