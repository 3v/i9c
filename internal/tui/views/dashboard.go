package views

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/config"
	"i9c/internal/tui/theme"
)

type DashboardModel struct {
	cfg           *config.Config
	width, height int
	lastScan      time.Time
	driftCount    int
	resourceCount int
	profileCount  int
	activeProfile string
	backendMode   string
	cacheHealth   string
	runtime       string
	prereqStatus  string
	prereqSummary string
	watcherActive bool
}

func NewDashboardModel(cfg *config.Config) *DashboardModel {
	return &DashboardModel{
		cfg:           cfg,
		watcherActive: true,
	}
}

func (m *DashboardModel) Init() tea.Cmd {
	return nil
}

func (m *DashboardModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

type DashboardUpdateMsg struct {
	DriftCount    int
	ResourceCount int
	ProfileCount  int
	ActiveProfile string
	BackendMode   string
	CacheHealth   string
	Runtime       string
	PrereqStatus  string
	PrereqSummary string
	LastScan      time.Time
}

func (m *DashboardModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case DashboardUpdateMsg:
		m.driftCount = msg.DriftCount
		m.resourceCount = msg.ResourceCount
		m.profileCount = msg.ProfileCount
		m.activeProfile = msg.ActiveProfile
		if msg.BackendMode != "" {
			m.backendMode = msg.BackendMode
		}
		if msg.CacheHealth != "" {
			m.cacheHealth = msg.CacheHealth
		}
		if msg.Runtime != "" {
			m.runtime = msg.Runtime
		}
		if msg.PrereqStatus != "" {
			m.prereqStatus = msg.PrereqStatus
		}
		if msg.PrereqSummary != "" {
			m.prereqSummary = msg.PrereqSummary
		}
		m.lastScan = msg.LastScan
	}
	return nil
}

func (m *DashboardModel) View() string {
	driftCard := m.renderCard("Drift", fmt.Sprintf("%d", m.driftCount), m.driftStatusColor())
	resourceCard := m.renderCard("Resources", fmt.Sprintf("%d", m.resourceCount), theme.Secondary)
	profileCountCard := m.renderCard("Profiles", fmt.Sprintf("%d", m.profileCount), theme.Primary)

	watcherStatus := "Active"
	watcherColor := theme.Success
	if !m.watcherActive {
		watcherStatus = "Stopped"
		watcherColor = theme.Danger
	}
	watcherCard := m.renderCard("Watcher", watcherStatus, watcherColor)
	profileText := m.activeProfile
	if profileText == "" {
		profileText = "(none)"
	}
	activeProfileCard := m.renderCard("Active Profile", profileText, theme.Primary)
	backend := m.backendMode
	if backend == "" {
		backend = "local"
	}
	backendCard := m.renderCard("Backend", backend, theme.Secondary)
	cache := m.cacheHealth
	if cache == "" {
		cache = "ok"
	}
	cacheCard := m.renderCard("Cache", cache, theme.Success)
	runtimeValue := m.runtime
	if runtimeValue == "" {
		runtimeValue = "unknown"
	}
	runtimeCard := m.renderCard("Runtime", runtimeValue, theme.TextDim)
	prereqStatus := m.prereqStatus
	if prereqStatus == "" {
		prereqStatus = "unknown"
	}
	prereqColor := theme.Success
	if strings.HasPrefix(prereqStatus, "WARN") {
		prereqColor = theme.Warning
	} else if strings.HasPrefix(prereqStatus, "MISSING") {
		prereqColor = theme.Danger
	}
	prereqCard := m.renderCard("Prereqs", prereqStatus, prereqColor)

	lastScanText := "Never"
	if !m.lastScan.IsZero() {
		lastScanText = m.lastScan.Format("15:04:05")
	}
	scanCard := m.renderCard("Last Scan", lastScanText, theme.TextDim)

	iacCard := m.renderCard("IaC Dir", m.cfg.IACDir, theme.TextDim)

	row1 := lipgloss.JoinHorizontal(lipgloss.Top, driftCard, "  ", resourceCard, "  ", profileCountCard)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top, watcherCard, "  ", scanCard, "  ", iacCard)
	row3 := lipgloss.JoinHorizontal(lipgloss.Top, activeProfileCard, "  ", backendCard, "  ", cacheCard)
	row4 := lipgloss.JoinHorizontal(lipgloss.Top, runtimeCard, "  ", prereqCard)

	title := theme.TitleStyle.Render("Dashboard")
	subtitle := lipgloss.NewStyle().Foreground(theme.TextDim).Render("  Overview of your infrastructure state")

	content := strings.Join([]string{
		title + subtitle,
		"",
		row1,
		"",
		row2,
		"",
		row3,
		"",
		row4,
		"",
		lipgloss.NewStyle().Foreground(theme.TextDim).Render("Prereq Details: "+m.prereqSummary),
		"",
		m.renderQuickInfo(),
	}, "\n")

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Padding(1, 2).
		Render(content)
}

func (m *DashboardModel) renderCard(title, value string, color lipgloss.Color) string {
	t := theme.CardTitleStyle.Render(title)
	v := lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
	content := t + "\n" + v
	return theme.CardStyle.Copy().Render(content)
}

func (m *DashboardModel) driftStatusColor() lipgloss.Color {
	if m.driftCount == 0 {
		return theme.Success
	}
	if m.driftCount < 5 {
		return theme.Warning
	}
	return theme.Danger
}

func (m *DashboardModel) renderQuickInfo() string {
	k := theme.HelpKeyStyle.Render
	d := theme.HelpDescStyle.Render
	lines := []string{
		lipgloss.NewStyle().Foreground(theme.TextDim).Bold(true).Render("Quick Actions"),
		"",
		k("[2]") + d(" View drift details"),
		k("[3]") + d(" Ask the AI advisor"),
		k("[4]") + d(" Browse AWS resources"),
		k("[/]") + d(" Search and filter"),
	}
	return strings.Join(lines, "\n")
}
