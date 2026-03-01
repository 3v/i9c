package views

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/aws"
	"i9c/internal/tui/theme"
)

type ProfileCloseMsg struct{}
type ProfileSelectMsg struct{ Profile string }
type ProfileLoginMsg struct{ Profile string }
type ProfileRefreshMsg struct{}
type ProfileCancelLoginMsg struct{}
type ProfileActionStatusMsg struct {
	Text    string
	IsError bool
}

type ProfileRow struct {
	Name      string
	AuthType  string
	Region    string
	Status    string
	AccountID string
	Session   string
	IsActive  bool
}

type ProfilesModel struct {
	width, height int
	rows          []ProfileRow
	filtered      []ProfileRow
	cursor        int
	filterMode    bool
	filterText    string
	statusText    string
	statusError   bool
	statusAt      time.Time
}

func NewProfilesModel() *ProfilesModel { return &ProfilesModel{} }

func (m *ProfilesModel) SetSize(w, h int) { m.width, m.height = w, h }

func (m *ProfilesModel) SetRows(rows []ProfileRow) {
	m.rows = rows
	m.applyFilter()
}

func (m *ProfilesModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ProfileActionStatusMsg:
		m.statusText = msg.Text
		m.statusError = msg.IsError
		m.statusAt = time.Now()
		return nil
	case tea.KeyMsg:
		if m.filterMode {
			switch msg.String() {
			case "enter", "esc":
				m.filterMode = false
				if msg.String() == "esc" {
					m.filterText = ""
				}
				m.applyFilter()
			case "backspace":
				if len(m.filterText) > 0 {
					m.filterText = m.filterText[:len(m.filterText)-1]
				}
				m.applyFilter()
			default:
				if len(msg.String()) == 1 {
					m.filterText += msg.String()
				}
				m.applyFilter()
			}
			return nil
		}

		switch msg.String() {
		case "esc", "p":
			return func() tea.Msg { return ProfileCloseMsg{} }
		case "/":
			m.filterMode = true
		case "j", "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "r":
			m.statusText = "Refresh requested..."
			m.statusError = false
			m.statusAt = time.Now()
			return func() tea.Msg { return ProfileRefreshMsg{} }
		case "x":
			m.statusText = "Cancel login requested..."
			m.statusError = false
			m.statusAt = time.Now()
			return func() tea.Msg { return ProfileCancelLoginMsg{} }
		case "l":
			if m.cursor < len(m.filtered) {
				return func() tea.Msg { return ProfileLoginMsg{Profile: m.filtered[m.cursor].Name} }
			}
		case "enter":
			if m.cursor < len(m.filtered) {
				row := m.filtered[m.cursor]
				switch aws.SessionStatus(row.Status) {
				case aws.StatusLive:
					return func() tea.Msg { return ProfileSelectMsg{Profile: row.Name} }
				default:
					return func() tea.Msg { return ProfileLoginMsg{Profile: row.Name} }
				}
			}
		}
	}
	return nil
}

func (m *ProfilesModel) applyFilter() {
	if m.filterText == "" {
		m.filtered = append([]ProfileRow(nil), m.rows...)
	} else {
		q := strings.ToLower(m.filterText)
		out := make([]ProfileRow, 0, len(m.rows))
		for _, r := range m.rows {
			if strings.Contains(strings.ToLower(r.Name), q) ||
				strings.Contains(strings.ToLower(r.Status), q) ||
				strings.Contains(strings.ToLower(r.Region), q) {
				out = append(out, r)
			}
		}
		m.filtered = out
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *ProfilesModel) View() string {
	title := theme.TitleStyle.Render("AWS Profiles")
	subtitle := lipgloss.NewStyle().Foreground(theme.TextDim).Render("  Select profile, login if needed")
	if len(m.filtered) == 0 {
		return lipgloss.NewStyle().Width(m.width).Height(m.height).Padding(1, 2).
			Render(title + subtitle + "\n\nNo profiles. Press [r] to refresh.")
	}

	activeW, profileW, authW := 6, 29, 8
	regionW, statusW, accountW := 12, 10, 14
	sep := "   "
	fixed := 2 + activeW + len(sep) + profileW + len(sep) + authW + len(sep) + regionW + len(sep) + statusW + len(sep) + accountW + len(sep)
	sessionW := m.width - 6 - fixed
	if sessionW < 8 {
		sessionW = 8
	}

	headerCells := []string{
		fmt.Sprintf("%-*s", activeW, "ACTIVE"),
		fmt.Sprintf("%-*s", profileW, "PROFILE"),
		fmt.Sprintf("%-*s", authW, "AUTH"),
		fmt.Sprintf("%-*s", regionW, "REGION"),
		fmt.Sprintf("%-*s", statusW, "STATUS"),
		fmt.Sprintf("%-*s", accountW, "ACCOUNT"),
		fmt.Sprintf("%-*s", sessionW, "SESSION"),
	}
	header := theme.TableHeaderStyle.Render("  " + strings.Join(headerCells, sep))

	visible := m.height - 8
	if visible < 1 {
		visible = 1
	}
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	end := start + visible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		r := m.filtered[i]
		active := ""
		if r.IsActive {
			active = "*"
		}
		cells := []string{
			fmt.Sprintf("%-*s", activeW, truncateFit(active, activeW)),
			fmt.Sprintf("%-*s", profileW, truncateFit(r.Name, profileW)),
			fmt.Sprintf("%-*s", authW, truncateFit(r.AuthType, authW)),
			fmt.Sprintf("%-*s", regionW, truncateFit(r.Region, regionW)),
			fmt.Sprintf("%-*s", statusW, truncateFit(r.Status, statusW)),
			fmt.Sprintf("%-*s", accountW, truncateFit(r.AccountID, accountW)),
			fmt.Sprintf("%-*s", sessionW, truncateFit(r.Session, sessionW)),
		}
		line := "  " + strings.Join(cells, sep)
		if i == m.cursor {
			line = lipgloss.NewStyle().Background(theme.Primary).Foreground(theme.BgDark).Bold(true).Render(line)
		}
		rows = append(rows, line)
	}

	hint := "[j/k] navigate  [enter] select/login  [l] login  [x] cancel login  [r] refresh  [/] filter  [esc] close"
	if m.filterMode {
		hint = fmt.Sprintf("Filter: %s█ (enter/esc to close)", m.filterText)
	}
	statusLine := ""
	if m.statusText != "" && time.Since(m.statusAt) < 5*time.Second {
		st := lipgloss.NewStyle().Foreground(theme.Success).Render("Status: " + m.statusText)
		if m.statusError {
			st = lipgloss.NewStyle().Foreground(theme.Danger).Render("Status: " + m.statusText)
		}
		statusLine = st
	}
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Padding(1, 2).
		Render(strings.Join([]string{title + subtitle, "", header, strings.Join(rows, "\n"), "", hint, statusLine}, "\n"))
}

func truncateFit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return string(r[:1])
	}
	return string(r[:width-1]) + "…"
}
