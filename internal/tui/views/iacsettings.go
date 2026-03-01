package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/config"
	"i9c/internal/iac"
	"i9c/internal/tui/theme"
)

type IaCSettingsSaveMsg struct {
	Binary   string
	Version  string
	AutoInit bool
}

type IaCSettingsCancelMsg struct{}

var iacTools = []struct {
	Binary  string
	Display string
}{
	{"tofu", "opentofu (tofu)"},
	{"terraform", "terraform"},
}

const (
	iacFieldVersion  = 0
	iacFieldAutoInit = 1
	iacNumFields     = 2
)

type IaCSettingsModel struct {
	width, height int

	cursor       int
	selectedTool int
	autoInit     bool

	editing    int
	textInputs [1]textinput.Model

	resolvedPath    string
	resolvedVersion string
}

func NewIaCSettingsModel(cfg config.TerraformConfig) *IaCSettingsModel {
	toolIdx := 0
	for i, t := range iacTools {
		if t.Binary == cfg.Binary {
			toolIdx = i
			break
		}
	}

	m := &IaCSettingsModel{
		selectedTool: toolIdx,
		autoInit:     cfg.AutoInit,
		editing:      -1,
	}

	tiVersion := textinput.New()
	tiVersion.Prompt = ""
	tiVersion.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	tiVersion.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Primary)
	tiVersion.CharLimit = 64
	ver := cfg.Version
	if ver == "" {
		ver = "latest"
	}
	tiVersion.SetValue(ver)
	m.textInputs = [1]textinput.Model{tiVersion}

	resolved, err := iac.InstalledVersion(cfg.Binary)
	if err == nil {
		m.resolvedPath = cfg.Binary
		m.resolvedVersion = resolved
	}

	return m
}

func (m *IaCSettingsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	inputW := max(20, w-20)
	m.textInputs[0].Width = inputW
}

func (m *IaCSettingsModel) totalRows() int {
	return len(iacTools) + iacNumFields
}

func (m *IaCSettingsModel) isToolRow() bool {
	return m.cursor < len(iacTools)
}

func (m *IaCSettingsModel) fieldIndex() int {
	return m.cursor - len(iacTools)
}

func (m *IaCSettingsModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.editing >= 0 {
			return m.handleEditingKey(msg)
		}
		return m.handleNavigationKey(msg)
	}
	return nil
}

func (m *IaCSettingsModel) handleEditingKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyEnter:
		m.textInputs[m.editing].Blur()
		m.editing = -1
		return nil
	}
	var cmd tea.Cmd
	m.textInputs[m.editing], cmd = m.textInputs[m.editing].Update(msg)
	return cmd
}

func (m *IaCSettingsModel) handleNavigationKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "t":
		return func() tea.Msg { return IaCSettingsCancelMsg{} }
	case "j", "down":
		if m.cursor < m.totalRows()-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter", " ":
		if m.isToolRow() {
			m.selectedTool = m.cursor
		} else {
			fi := m.fieldIndex()
			if fi == iacFieldVersion {
				m.editing = 0
				m.textInputs[0].Focus()
			} else if fi == iacFieldAutoInit {
				m.autoInit = !m.autoInit
			}
		}
	case "s":
		return m.save()
	case "G":
		m.cursor = m.totalRows() - 1
	case "g":
		m.cursor = 0
	}
	return nil
}

func (m *IaCSettingsModel) save() tea.Cmd {
	return func() tea.Msg {
		return IaCSettingsSaveMsg{
			Binary:   iacTools[m.selectedTool].Binary,
			Version:  strings.TrimSpace(m.textInputs[0].Value()),
			AutoInit: m.autoInit,
		}
	}
}

func (m *IaCSettingsModel) View() string {
	title := theme.TitleStyle.Render("IaC Settings")
	subtitle := lipgloss.NewStyle().Foreground(theme.TextDim).Render("  Configure infrastructure-as-code tool")

	divider := lipgloss.NewStyle().
		Foreground(theme.Border).
		Render(strings.Repeat("─", min(m.width-6, 60)))

	toolLabel := lipgloss.NewStyle().Foreground(theme.Secondary).Bold(true).Render("TOOL")

	var toolRows []string
	for i, t := range iacTools {
		radio := "( )"
		radioStyle := theme.ProfileInactiveStyle
		if i == m.selectedTool {
			radio = "(•)"
			radioStyle = theme.ProfileActiveStyle
		}

		line := fmt.Sprintf("    %s %s", radioStyle.Render(radio), lipgloss.NewStyle().Foreground(theme.Text).Render(t.Display))

		if i == m.cursor {
			line = fmt.Sprintf("    %s %s",
				lipgloss.NewStyle().Foreground(theme.BgDark).Bold(true).Render(radio),
				lipgloss.NewStyle().Foreground(theme.BgDark).Bold(true).Render(t.Display),
			)
			line = lipgloss.NewStyle().Background(theme.Primary).Render(line)
		}
		toolRows = append(toolRows, line)
	}

	configLabel := lipgloss.NewStyle().Foreground(theme.Secondary).Bold(true).Render("CONFIGURATION")

	var fieldRows []string

	// Version field
	{
		rowIdx := len(iacTools) + iacFieldVersion
		label := lipgloss.NewStyle().Width(10).Foreground(theme.TextDim).Render("VERSION")
		var value string
		if m.editing == 0 {
			value = m.textInputs[0].View()
		} else {
			raw := m.textInputs[0].Value()
			if raw == "" {
				value = lipgloss.NewStyle().Foreground(theme.Muted).Render("latest")
			} else {
				value = lipgloss.NewStyle().Foreground(theme.Text).Render(raw)
			}
		}
		hint := ""
		if m.editing != 0 {
			hint = lipgloss.NewStyle().Foreground(theme.Muted).Render("  [enter] edit")
		}
		line := fmt.Sprintf("    %s  %s%s", label, value, hint)
		if rowIdx == m.cursor && m.editing < 0 {
			line = lipgloss.NewStyle().Background(theme.Primary).Foreground(theme.BgDark).Bold(true).
				Render(fmt.Sprintf("    %-10s  %s", "VERSION", m.textInputs[0].Value()))
		}
		fieldRows = append(fieldRows, line)
	}

	// AutoInit field
	{
		rowIdx := len(iacTools) + iacFieldAutoInit
		label := lipgloss.NewStyle().Width(10).Foreground(theme.TextDim).Render("AUTO INIT")
		checkbox := "[ ]"
		if m.autoInit {
			checkbox = "[x]"
		}
		value := lipgloss.NewStyle().Foreground(theme.Text).Render(checkbox)
		hint := lipgloss.NewStyle().Foreground(theme.Muted).Render("  [enter] toggle")
		line := fmt.Sprintf("    %s  %s%s", label, value, hint)
		if rowIdx == m.cursor && m.editing < 0 {
			line = lipgloss.NewStyle().Background(theme.Primary).Foreground(theme.BgDark).Bold(true).
				Render(fmt.Sprintf("    %-10s  %s", "AUTO INIT", checkbox))
		}
		fieldRows = append(fieldRows, line)
	}

	resolvedLine := ""
	if m.resolvedVersion != "" {
		resolvedLine = lipgloss.NewStyle().Foreground(theme.Secondary).
			Render(fmt.Sprintf("  Resolved: %s (%s)", m.resolvedPath, m.resolvedVersion))
	}

	hint := lipgloss.NewStyle().Foreground(theme.TextDim).Render(
		"[j/k] navigate  [enter/space] select  [s] save  [esc] cancel")

	var lines []string
	lines = append(lines, title+subtitle)
	lines = append(lines, "")
	lines = append(lines, "  "+toolLabel)
	lines = append(lines, divider)
	lines = append(lines, toolRows...)
	lines = append(lines, "")
	lines = append(lines, "  "+configLabel)
	lines = append(lines, divider)
	lines = append(lines, fieldRows...)
	lines = append(lines, "")
	if resolvedLine != "" {
		lines = append(lines, resolvedLine)
	}
	lines = append(lines, divider)
	lines = append(lines, hint)

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}
