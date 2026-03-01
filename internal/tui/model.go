package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/aws"
	"i9c/internal/config"
	ilog "i9c/internal/logs"
	"i9c/internal/tui/theme"
	"i9c/internal/tui/views"
)

type TabIndex int

const (
	TabDashboard TabIndex = iota
	TabDrift
	TabAdvisor
	TabResources
	TabLogs
	TabMCP
	TabProfiles
	TabFolderPicker
	TabLLMSettings
	TabIaCSettings
)

var tabNames = []string{"Dashboard", "Drift", "Advisor", "Resources", "Logs", "MCP"}

type AWSAction interface{}
type ProfileSelectRequest struct{ Profile string }
type ProfileLoginRequest struct{ Profile string }
type ProfileRefreshRequest struct{}
type ProfileCancelLoginRequest struct{}

type ProfileCatalogMsg struct {
	Profiles      []aws.ProfileInfo
	ActiveProfile string
}

type Model struct {
	cfg               *config.Config
	activeTab         TabIndex
	previousTab       TabIndex
	swallowAdvisorKey string
	width, height     int
	showHelp          bool
	filterMode        bool
	filterText        string
	dirChangeNotify   chan string
	advisorSendCh     chan string
	advisorCancelCh   chan struct{}
	llmConfigChangeCh chan config.LLMConfig
	iacConfigChangeCh chan config.TerraformConfig
	awsActionCh       chan AWSAction

	dashboard    *views.DashboardModel
	drift        *views.DriftModel
	advisor      *views.AdvisorModel
	resources    *views.ResourcesModel
	logs         *views.LogsModel
	mcp          *views.MCPModel
	profiles     *views.ProfilesModel
	folderPicker *views.FolderPickerModel
	llmSettings  *views.LLMSettingsModel
	iacSettings  *views.IaCSettingsModel
}

func NewModel(cfg *config.Config, hub *ilog.Hub) *Model {
	m := &Model{
		cfg:       cfg,
		activeTab: TabDashboard,
		dashboard: views.NewDashboardModel(cfg),
		drift:     views.NewDriftModel(cfg),
		advisor:   views.NewAdvisorModel(cfg),
		resources: views.NewResourcesModel(cfg),
		logs:      views.NewLogsModel(hub),
		mcp:       views.NewMCPModel(),
		profiles:  views.NewProfilesModel(),
	}
	if cfg.IACDir == "" {
		m.previousTab = TabDashboard
		m.folderPicker = views.NewFolderPickerModel(".")
		m.activeTab = TabFolderPicker
	}
	return m
}

func (m *Model) SetDirChangeNotify(ch chan string)                   { m.dirChangeNotify = ch }
func (m *Model) SetAdvisorSendCh(ch chan string)                     { m.advisorSendCh = ch }
func (m *Model) SetAdvisorCancelCh(ch chan struct{})                 { m.advisorCancelCh = ch }
func (m *Model) SetLLMConfigChangeCh(ch chan config.LLMConfig)       { m.llmConfigChangeCh = ch }
func (m *Model) SetIaCConfigChangeCh(ch chan config.TerraformConfig) { m.iacConfigChangeCh = ch }
func (m *Model) SetAWSActionCh(ch chan AWSAction)                    { m.awsActionCh = ch }

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.dashboard.Init(), m.drift.Init(), m.resources.Init())
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		contentHeight := m.height - 4
		m.dashboard.SetSize(m.width-2, contentHeight)
		m.drift.SetSize(m.width-2, contentHeight)
		m.advisor.SetSize(m.width-2, contentHeight)
		m.resources.SetSize(m.width-2, contentHeight)
		m.logs.SetSize(m.width-2, contentHeight)
		m.mcp.SetSize(m.width-2, contentHeight)
		m.profiles.SetSize(m.width-2, contentHeight)
		if m.folderPicker != nil {
			m.folderPicker.SetSize(m.width-2, contentHeight)
		}
		if m.llmSettings != nil {
			m.llmSettings.SetSize(m.width-2, contentHeight)
		}
		if m.iacSettings != nil {
			m.iacSettings.SetSize(m.width-2, contentHeight)
		}
		return m, nil
	case ProfileCatalogMsg:
		rows := make([]views.ProfileRow, 0, len(msg.Profiles))
		for _, p := range msg.Profiles {
			rows = append(rows, views.ProfileRow{Name: p.Name, AuthType: string(p.AuthType), Region: p.Region, Status: string(p.Status), AccountID: p.AccountID, Session: p.SessionRef, IsActive: p.Name == msg.ActiveProfile})
		}
		m.profiles.SetRows(rows)
		m.dashboard.Update(views.DashboardUpdateMsg{ActiveProfile: msg.ActiveProfile, ProfileCount: len(msg.Profiles)})
		return m, nil
	case views.ProfileSelectMsg:
		if m.awsActionCh != nil {
			go func() { m.awsActionCh <- ProfileSelectRequest{Profile: msg.Profile} }()
		}
		return m, nil
	case views.ProfileLoginMsg:
		if m.awsActionCh != nil {
			go func() { m.awsActionCh <- ProfileLoginRequest{Profile: msg.Profile} }()
		}
		return m, nil
	case views.ProfileRefreshMsg:
		if m.awsActionCh != nil {
			go func() { m.awsActionCh <- ProfileRefreshRequest{} }()
		}
		return m, nil
	case views.ProfileCancelLoginMsg:
		if m.awsActionCh != nil {
			go func() { m.awsActionCh <- ProfileCancelLoginRequest{} }()
		}
		return m, nil
	case views.ProfileActionStatusMsg:
		return m, m.profiles.Update(msg)
	case views.ProfileCloseMsg:
		m.activeTab = m.previousTab
		return m, nil
	case views.FolderSelectedMsg:
		m.cfg.IACDir = msg.Path
		m.activeTab = m.previousTab
		m.folderPicker = nil
		if m.dirChangeNotify != nil {
			go func() { m.dirChangeNotify <- msg.Path }()
		}
		_ = m.cfg.Save("")
		return m, nil
	case views.LLMSettingsSaveMsg:
		m.cfg.LLM.Provider, m.cfg.LLM.Model, m.cfg.LLM.APIKey, m.cfg.LLM.BaseURL = msg.Provider, msg.Model, msg.APIKey, msg.BaseURL
		_ = m.cfg.Save("")
		if m.llmConfigChangeCh != nil {
			newCfg := m.cfg.LLM
			go func() { m.llmConfigChangeCh <- newCfg }()
		}
		m.activeTab, m.llmSettings = m.previousTab, nil
		return m, nil
	case views.LLMSettingsCancelMsg:
		m.activeTab, m.llmSettings = m.previousTab, nil
		return m, nil
	case views.IaCSettingsSaveMsg:
		m.cfg.Terraform.Binary, m.cfg.Terraform.Version, m.cfg.Terraform.AutoInit = msg.Binary, msg.Version, msg.AutoInit
		_ = m.cfg.Save("")
		if m.iacConfigChangeCh != nil {
			newCfg := m.cfg.Terraform
			go func() { m.iacConfigChangeCh <- newCfg }()
		}
		m.activeTab, m.iacSettings = m.previousTab, nil
		return m, nil
	case views.IaCSettingsCancelMsg:
		m.activeTab, m.iacSettings = m.previousTab, nil
		return m, nil
	case views.FolderCancelMsg:
		m.activeTab, m.folderPicker = m.previousTab, nil
		return m, nil
	case views.AdvisorSendMsg:
		if m.advisorSendCh != nil {
			go func() { m.advisorSendCh <- msg.Text }()
		}
		return m, nil
	case views.AdvisorCancelMsg:
		if m.advisorCancelCh != nil {
			go func() { m.advisorCancelCh <- struct{}{} }()
		}
		return m, nil
	case views.AdvisorResponseMsg:
		return m, m.advisor.Update(msg)
	case views.AdvisorBusyMsg:
		return m, m.advisor.Update(msg)
	case views.AdvisorActivityMsg:
		return m, m.advisor.Update(msg)
	case views.DashboardUpdateMsg:
		m.dashboard.Update(msg)
		return m, nil
	case views.DriftUpdateMsg:
		m.drift.Update(msg)
		return m, nil
	case views.ResourcesUpdateMsg:
		m.resources.Update(msg)
		return m, nil
	case views.MCPUpdateMsg:
		m.mcp.Update(msg)
		return m, nil
	case tea.KeyMsg:
		if m.activeTab == TabAdvisor && m.swallowAdvisorKey != "" {
			if msg.String() == m.swallowAdvisorKey {
				m.swallowAdvisorKey = ""
				return m, nil
			}
			m.swallowAdvisorKey = ""
		}
		switch m.activeTab {
		case TabProfiles:
			return m, m.profiles.Update(msg)
		case TabFolderPicker:
			if m.folderPicker != nil {
				return m, m.folderPicker.Update(msg)
			}
		case TabLLMSettings:
			if m.llmSettings != nil {
				return m, m.llmSettings.Update(msg)
			}
		case TabIaCSettings:
			if m.iacSettings != nil {
				return m, m.iacSettings.Update(msg)
			}
		}
		if m.filterMode {
			return m.handleFilterKey(msg)
		}
		if m.advisor.IsInputActive() && m.activeTab == TabAdvisor {
			return m.updateActiveView(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "ctrl+a":
			m.filterMode = false
			m.filterText = ""
			m.showHelp = false
			m.activeTab = TabDashboard
			return m, nil
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
		case "?":
			m.showHelp = !m.showHelp
		case "/":
			m.filterMode, m.filterText = true, ""
		case "p":
			m.previousTab, m.activeTab = m.activeTab, TabProfiles
		case "f":
			m.previousTab, m.activeTab = m.activeTab, TabFolderPicker
			m.folderPicker = views.NewFolderPickerModel(m.cfg.IACDir)
			m.folderPicker.SetSize(m.width-2, m.height-4)
		case "l":
			m.previousTab, m.activeTab = m.activeTab, TabLLMSettings
			m.llmSettings = views.NewLLMSettingsModel(m.cfg.LLM)
			m.llmSettings.SetSize(m.width-2, m.height-4)
		case "t":
			m.previousTab, m.activeTab = m.activeTab, TabIaCSettings
			m.iacSettings = views.NewIaCSettingsModel(m.cfg.Terraform)
			m.iacSettings.SetSize(m.width-2, m.height-4)
		case "1":
			m.activeTab = TabDashboard
			return m, nil
		case "2":
			m.activeTab = TabDrift
			return m, nil
		case "3":
			m.activeTab = TabAdvisor
			m.swallowAdvisorKey = "3"
			return m, nil
		case "4":
			m.activeTab = TabResources
			return m, nil
		case "5":
			m.activeTab = TabLogs
			return m, nil
		case "6":
			m.activeTab = TabMCP
			return m, nil
		case "tab":
			if m.activeTab <= TabMCP {
				m.activeTab = (m.activeTab + 1) % 6
			}
			return m, nil
		case "shift+tab":
			if m.activeTab <= TabMCP {
				m.activeTab = (m.activeTab + 6 - 1) % 6
			}
			return m, nil
		}
	}
	return m.updateActiveView(msg)
}

func (m *Model) updateActiveView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.activeTab {
	case TabDashboard:
		return m, m.dashboard.Update(msg)
	case TabDrift:
		return m, m.drift.Update(msg)
	case TabAdvisor:
		return m, m.advisor.Update(msg)
	case TabResources:
		return m, m.resources.Update(msg)
	case TabLogs:
		return m, m.logs.Update(msg)
	case TabMCP:
		return m, m.mcp.Update(msg)
	}
	return m, nil
}

func (m *Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.filterMode = false
		if msg.String() == "esc" {
			m.filterText = ""
		}
		m.drift.SetFilter(m.filterText)
		m.resources.SetFilter(m.filterText)
	case "backspace":
		if len(m.filterText) > 0 {
			m.filterText = m.filterText[:len(m.filterText)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.filterText += msg.String()
		}
	}
	return m, nil
}

func (m *Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	header, content, status := m.renderHeader(), m.renderContent(), m.renderStatusBar()
	if m.showHelp {
		content = m.renderHelp()
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, content, status)
}

func (m *Model) renderHeader() string {
	title := theme.TitleStyle.Render("i9c")
	subtitle := lipgloss.NewStyle().Foreground(theme.TextDim).Render(" Infrastructure Advisor")
	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf("[%d] %s", i+1, name)
		if TabIndex(i) == m.activeTab {
			tabs = append(tabs, theme.TabActiveStyle.Render(label))
		} else {
			tabs = append(tabs, theme.TabInactiveStyle.Render(label))
		}
	}
	if m.activeTab == TabProfiles {
		tabs = append(tabs, theme.TabActiveStyle.Render("[p] Profiles"))
	}
	if m.activeTab == TabLLMSettings {
		tabs = append(tabs, theme.TabActiveStyle.Render("[l] LLM"))
	}
	if m.activeTab == TabIaCSettings {
		tabs = append(tabs, theme.TabActiveStyle.Render("[t] IaC"))
	}
	left, right := title+subtitle, strings.Join(tabs, " ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return lipgloss.NewStyle().Width(m.width).Background(theme.BgPanel).Render(left + strings.Repeat(" ", gap) + right)
}

func (m *Model) renderContent() string {
	contentHeight := max(1, m.height-4)
	var content string
	switch m.activeTab {
	case TabDashboard:
		content = m.dashboard.View()
	case TabDrift:
		content = m.drift.View()
	case TabAdvisor:
		content = m.advisor.View()
	case TabResources:
		content = m.resources.View()
	case TabLogs:
		content = m.logs.View()
	case TabMCP:
		content = m.mcp.View()
	case TabProfiles:
		content = m.profiles.View()
	case TabFolderPicker:
		if m.folderPicker != nil {
			content = m.folderPicker.View()
		}
	case TabLLMSettings:
		if m.llmSettings != nil {
			content = m.llmSettings.View()
		}
	case TabIaCSettings:
		if m.iacSettings != nil {
			content = m.iacSettings.View()
		}
	}
	return lipgloss.NewStyle().Width(m.width).Height(contentHeight).Render(content)
}

func (m *Model) renderStatusBar() string {
	k, d := theme.StatusBarKeyStyle.Render, theme.HelpDescStyle.Render
	left := strings.Join([]string{
		k("[?]") + d(" Help"),
		k("[q]") + d(" Quit"),
		k("[/]") + d(" Filter"),
		k("[p]") + d(" Profiles"),
		k("[f]") + d(" Folder"),
		k("[l]") + d(" LLM"),
		k("[t]") + d(" IaC"),
		k("[5]") + d(" Logs"),
		k("[6]") + d(" MCP"),
		k("[tab]") + d(" Next"),
	}, "  ")
	right := d("Watching: ") + theme.StatusBarKeyStyle.Render(m.cfg.IACDir)
	if m.filterMode {
		right = k("Filter: ") + theme.ChatUserStyle.Render(m.filterText+"█")
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return theme.StatusBarStyle.Width(m.width).Render(left + strings.Repeat(" ", gap) + right)
}

func (m *Model) renderHelp() string {
	k, d := theme.HelpKeyStyle.Render, theme.HelpDescStyle.Render
	lines := []string{
		theme.TitleStyle.Render("Keyboard Shortcuts"),
		"",
		k("1-6") + d("     Switch panels"),
		k("tab") + d("     Next panel"),
		k("shift+tab") + d(" Prev panel"),
		k("j/k") + d("     Navigate"),
		k("enter") + d("   Select"),
		k("p") + d("       Profile picker"),
		k("l") + d("       LLM settings"),
		k("f") + d("       Folder picker"),
		k("t") + d("       IaC settings"),
		k("/") + d("       Global filter"),
		k("q") + d("       Quit"),
	}
	return lipgloss.NewStyle().Width(m.width).Height(max(1, m.height-4)).Padding(1, 2).Render(strings.Join(lines, "\n"))
}
