package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/config"
	"i9c/internal/tui/theme"
)

type ResourceEntry struct {
	Profile    string
	Service    string
	Type       string
	ID         string
	Name       string
	Region     string
	Properties map[string]string
}

type ResourcesModel struct {
	cfg           *config.Config
	width, height int
	entries       []ResourceEntry
	filtered      []ResourceEntry
	cursor        int
	filterText    string
	profileFilter map[string]bool
	detailMode    bool
	serviceFilter string
}

func NewResourcesModel(cfg *config.Config) *ResourcesModel {
	return &ResourcesModel{
		cfg: cfg,
	}
}

func (m *ResourcesModel) Init() tea.Cmd {
	return nil
}

func (m *ResourcesModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *ResourcesModel) SetFilter(text string) {
	m.filterText = text
	m.applyFilter()
}

type ResourcesUpdateMsg struct {
	Entries []ResourceEntry
}

func (m *ResourcesModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ResourcesUpdateMsg:
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
		case "s":
			m.cycleServiceFilter()
		case "G":
			m.cursor = max(0, len(m.filtered)-1)
		case "g":
			m.cursor = 0
		}
	}
	return nil
}

func (m *ResourcesModel) View() string {
	if m.detailMode && len(m.filtered) > 0 {
		return m.renderDetail()
	}
	return m.renderTable()
}

func (m *ResourcesModel) renderTable() string {
	title := theme.TitleStyle.Render("AWS Resources")

	filterInfo := ""
	if m.serviceFilter != "" {
		filterInfo = lipgloss.NewStyle().Foreground(theme.Secondary).Render(
			fmt.Sprintf("  [s] Service: %s", m.serviceFilter))
	} else {
		filterInfo = lipgloss.NewStyle().Foreground(theme.TextDim).Render("  [s] filter by service")
	}

	if len(m.filtered) == 0 {
		msg := "No resources found"
		if len(m.entries) == 0 {
			msg = "Loading resources... Connect AWS profiles to browse resources."
		}
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Padding(1, 2).
			Render(title + filterInfo + "\n\n" + lipgloss.NewStyle().Foreground(theme.TextDim).Render(msg))
	}

	headerFmt := "%-14s %-10s %-24s %-24s %-20s %s"
	header := theme.TableHeaderStyle.Render(
		fmt.Sprintf(headerFmt, "PROFILE", "SERVICE", "TYPE", "ID", "NAME", "REGION"))

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
		line := fmt.Sprintf(headerFmt,
			truncate(e.Profile, 14),
			truncate(e.Service, 10),
			truncate(e.Type, 24),
			truncate(e.ID, 24),
			truncate(e.Name, 20),
			truncate(e.Region, 14),
		)
		if i == m.cursor {
			rows = append(rows, theme.TableSelectedStyle.Render(line))
		} else {
			rows = append(rows, theme.TableRowStyle.Render(line))
		}
	}

	counter := lipgloss.NewStyle().Foreground(theme.TextDim).Render(
		fmt.Sprintf("  %d resources", len(m.filtered)))

	table := strings.Join(append([]string{header}, rows...), "\n")

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Padding(1, 2).
		Render(title + counter + filterInfo + "\n\n" + table)
}

func (m *ResourcesModel) renderDetail() string {
	e := m.filtered[m.cursor]
	title := theme.TitleStyle.Render("Resource Detail")
	back := lipgloss.NewStyle().Foreground(theme.TextDim).Render("  [esc] back")

	lines := []string{
		title + back,
		"",
		theme.HelpKeyStyle.Render("Profile:  ") + e.Profile,
		theme.HelpKeyStyle.Render("Service:  ") + e.Service,
		theme.HelpKeyStyle.Render("Type:     ") + e.Type,
		theme.HelpKeyStyle.Render("ID:       ") + e.ID,
		theme.HelpKeyStyle.Render("Name:     ") + e.Name,
		theme.HelpKeyStyle.Render("Region:   ") + e.Region,
	}

	if len(e.Properties) > 0 {
		lines = append(lines, "", theme.HelpKeyStyle.Render("Properties:"))
		for k, v := range e.Properties {
			lines = append(lines, fmt.Sprintf("  %s: %s",
				lipgloss.NewStyle().Foreground(theme.Secondary).Render(k),
				v))
		}
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

func (m *ResourcesModel) SetProfileFilter(active map[string]bool) {
	m.profileFilter = active
	m.applyFilter()
}

func (m *ResourcesModel) ClearServiceFilter() {
	m.serviceFilter = ""
	m.filterText = ""
	m.applyFilter()
}

func (m *ResourcesModel) applyFilter() {
	var result []ResourceEntry
	for _, e := range m.entries {
		if m.profileFilter != nil && !m.profileFilter[e.Profile] {
			continue
		}
		if m.serviceFilter != "" && e.Service != m.serviceFilter {
			continue
		}
		if m.filterText != "" {
			lower := strings.ToLower(m.filterText)
			if !strings.Contains(strings.ToLower(e.ID), lower) &&
				!strings.Contains(strings.ToLower(e.Name), lower) &&
				!strings.Contains(strings.ToLower(e.Type), lower) &&
				!strings.Contains(strings.ToLower(e.Profile), lower) {
				continue
			}
		}
		result = append(result, e)
	}
	m.filtered = result
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m *ResourcesModel) cycleServiceFilter() {
	services := m.uniqueServices()
	if len(services) == 0 {
		return
	}
	if m.serviceFilter == "" {
		m.serviceFilter = services[0]
	} else {
		for i, s := range services {
			if s == m.serviceFilter {
				if i+1 < len(services) {
					m.serviceFilter = services[i+1]
				} else {
					m.serviceFilter = ""
				}
				break
			}
		}
	}
	m.applyFilter()
}

func (m *ResourcesModel) uniqueServices() []string {
	seen := make(map[string]bool)
	var result []string
	for _, e := range m.entries {
		if !seen[e.Service] {
			seen[e.Service] = true
			result = append(result, e.Service)
		}
	}
	return result
}
