package theme

import "github.com/charmbracelet/lipgloss"

var (
	Primary   = lipgloss.Color("#7C3AED")
	Secondary = lipgloss.Color("#06B6D4")
	Success   = lipgloss.Color("#22C55E")
	Warning   = lipgloss.Color("#F59E0B")
	Danger    = lipgloss.Color("#EF4444")
	Muted     = lipgloss.Color("#6B7280")
	Text      = lipgloss.Color("#E5E7EB")
	TextDim   = lipgloss.Color("#9CA3AF")
	BgDark    = lipgloss.Color("#111827")
	BgPanel   = lipgloss.Color("#1F2937")
	Border    = lipgloss.Color("#374151")

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary).
			PaddingLeft(1)

	TabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(BgDark).
			Background(Primary).
			Padding(0, 1)

	TabInactiveStyle = lipgloss.NewStyle().
				Foreground(TextDim).
				Padding(0, 1)

	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Border).
			Padding(0, 1)

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(TextDim).
			Background(BgPanel).
			Padding(0, 1)

	StatusBarKeyStyle = lipgloss.NewStyle().
				Foreground(Secondary).
				Bold(true)

	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(Secondary).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(Border)

	TableRowStyle = lipgloss.NewStyle().
			Foreground(Text)

	TableSelectedStyle = lipgloss.NewStyle().
				Foreground(BgDark).
				Background(Primary).
				Bold(true)

	DriftAddStyle = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)

	DriftChangeStyle = lipgloss.NewStyle().
				Foreground(Warning).
				Bold(true)

	DriftDestroyStyle = lipgloss.NewStyle().
				Foreground(Danger).
				Bold(true)

	ProfileActiveStyle = lipgloss.NewStyle().
				Foreground(Success)

	ProfileInactiveStyle = lipgloss.NewStyle().
				Foreground(Muted)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(TextDim)

	ChatUserStyle = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true)

	ChatAssistantStyle = lipgloss.NewStyle().
				Foreground(Success).
				Bold(true)

	CodeBlockStyle = lipgloss.NewStyle().
			Background(BgPanel).
			Foreground(Text).
			Padding(0, 1)

	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Border).
			Padding(1, 2).
			Width(24)

	CardTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Text)

	CardValueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary)
)
