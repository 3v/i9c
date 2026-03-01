package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/codexbridge"
	"i9c/internal/config"
	"i9c/internal/tui/theme"
)

type LLMSettingsSaveMsg struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

type LLMSettingsCancelMsg struct{}

var llmProviders = []string{"codex", "openai", "claude", "bedrock", "ollama"}

const (
	fieldModel   = 0
	fieldAPIKey  = 1
	fieldBaseURL = 2
	numFields    = 3
)

type LLMSettingsModel struct {
	width, height int

	cursor           int
	selectedProvider int

	editing           int
	textInputs        [numFields]textinput.Model
	modelOptions      []string
	modelDropdown     bool
	modelOptionCursor int

	origProvider string
	origModel    string
	origAPIKey   string
	origBaseURL  string
}

func NewLLMSettingsModel(cfg config.LLMConfig) *LLMSettingsModel {
	providerIdx := 0
	for i, p := range llmProviders {
		if p == cfg.Provider {
			providerIdx = i
			break
		}
	}

	m := &LLMSettingsModel{
		selectedProvider: providerIdx,
		editing:          -1,
		origProvider:     cfg.Provider,
		origModel:        cfg.Model,
		origAPIKey:       cfg.APIKey,
		origBaseURL:      cfg.BaseURL,
	}

	tiModel := textinput.New()
	tiModel.Prompt = ""
	tiModel.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	tiModel.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Primary)
	tiModel.CharLimit = 256
	tiModel.SetValue(cfg.Model)

	tiKey := textinput.New()
	tiKey.Prompt = ""
	tiKey.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	tiKey.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Primary)
	tiKey.CharLimit = 512
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = cfg.ResolveAPIKey()
	}
	tiKey.SetValue(apiKey)

	tiURL := textinput.New()
	tiURL.Prompt = ""
	tiURL.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	tiURL.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Primary)
	tiURL.CharLimit = 512
	tiURL.SetValue(cfg.BaseURL)

	m.textInputs = [numFields]textinput.Model{tiModel, tiKey, tiURL}
	m.setProvider(providerIdx)
	if strings.TrimSpace(m.textInputs[fieldModel].Value()) == "" && len(m.modelOptions) > 0 {
		m.textInputs[fieldModel].SetValue(m.modelOptions[0])
	}

	return m
}

func (m *LLMSettingsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	inputW := max(20, w-20)
	for i := range m.textInputs {
		m.textInputs[i].Width = inputW
	}
}

func (m *LLMSettingsModel) totalRows() int {
	return len(llmProviders) + numFields
}

func (m *LLMSettingsModel) isProviderRow() bool {
	return m.cursor < len(llmProviders)
}

func (m *LLMSettingsModel) fieldIndex() int {
	return m.cursor - len(llmProviders)
}

func (m *LLMSettingsModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.editing >= 0 {
			return m.handleEditingKey(msg)
		}
		return m.handleNavigationKey(msg)
	}
	return nil
}

func (m *LLMSettingsModel) handleEditingKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		m.textInputs[m.editing].Blur()
		m.editing = -1
		return nil
	case tea.KeyEnter:
		m.textInputs[m.editing].Blur()
		m.editing = -1
		return nil
	}
	var cmd tea.Cmd
	m.textInputs[m.editing], cmd = m.textInputs[m.editing].Update(msg)
	return cmd
}

func (m *LLMSettingsModel) handleNavigationKey(msg tea.KeyMsg) tea.Cmd {
	if m.modelDropdown {
		switch msg.String() {
		case "j", "down":
			if m.modelOptionCursor < len(m.modelOptions)-1 {
				m.modelOptionCursor++
			}
		case "k", "up":
			if m.modelOptionCursor > 0 {
				m.modelOptionCursor--
			}
		case "enter":
			if len(m.modelOptions) > 0 {
				m.textInputs[fieldModel].SetValue(m.modelOptions[m.modelOptionCursor])
			}
			m.modelDropdown = false
		case "esc":
			m.modelDropdown = false
		}
		return nil
	}

	switch msg.String() {
	case "esc", "l":
		return func() tea.Msg { return LLMSettingsCancelMsg{} }
	case "j", "down":
		if m.cursor < m.totalRows()-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter", " ":
		if m.isProviderRow() {
			m.setProvider(m.cursor)
		} else {
			fi := m.fieldIndex()
			if fi == fieldModel {
				if len(m.modelOptions) > 0 {
					m.modelDropdown = true
					m.modelOptionCursor = 0
					cur := m.textInputs[fieldModel].Value()
					for i, model := range m.modelOptions {
						if model == cur {
							m.modelOptionCursor = i
							break
						}
					}
				}
			} else {
				m.editing = fi
				m.textInputs[fi].Focus()
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

func (m *LLMSettingsModel) setProvider(idx int) {
	if idx < 0 || idx >= len(llmProviders) {
		return
	}
	m.selectedProvider = idx
	provider := llmProviders[idx]
	m.modelOptions = defaultModelOptions(provider)
	if provider == "codex" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if discovered, err := codexbridge.DiscoverModels(ctx, "codex"); err == nil && len(discovered) > 0 {
			m.modelOptions = discovered
		}
	}

	curModel := m.textInputs[fieldModel].Value()
	valid := false
	for _, model := range m.modelOptions {
		if model == curModel {
			valid = true
			break
		}
	}
	if !valid {
		if len(m.modelOptions) > 0 {
			m.textInputs[fieldModel].SetValue(m.modelOptions[0])
		} else if provider == "codex" {
			m.textInputs[fieldModel].SetValue("gpt-5")
		}
	}
}

func defaultModelOptions(provider string) []string {
	switch provider {
	case "codex":
		return []string{"gpt-5", "gpt-5-codex"}
	case "openai":
		return []string{"gpt-5", "gpt-4o", "gpt-4.1"}
	case "claude":
		return []string{"claude-3-5-sonnet-latest", "claude-3-7-sonnet-latest"}
	case "bedrock":
		return []string{"anthropic.claude-3-5-sonnet-20241022-v2:0"}
	case "ollama":
		return []string{"llama3.1", "qwen2.5-coder"}
	default:
		return nil
	}
}

func (m *LLMSettingsModel) save() tea.Cmd {
	return func() tea.Msg {
		return LLMSettingsSaveMsg{
			Provider: llmProviders[m.selectedProvider],
			Model:    strings.TrimSpace(m.textInputs[fieldModel].Value()),
			APIKey:   strings.TrimSpace(m.textInputs[fieldAPIKey].Value()),
			BaseURL:  strings.TrimSpace(m.textInputs[fieldBaseURL].Value()),
		}
	}
}

func (m *LLMSettingsModel) View() string {
	title := theme.TitleStyle.Render("LLM Settings")
	subtitle := lipgloss.NewStyle().Foreground(theme.TextDim).Render("  Configure AI provider for the Advisor panel")

	divider := lipgloss.NewStyle().Foreground(theme.Border).Render(strings.Repeat("─", min(m.width-6, 60)))
	providerLabel := lipgloss.NewStyle().Foreground(theme.Secondary).Bold(true).Render("PROVIDER")

	var providerRows []string
	for i, p := range llmProviders {
		radio := "( )"
		radioStyle := theme.ProfileInactiveStyle
		if i == m.selectedProvider {
			radio = "(•)"
			radioStyle = theme.ProfileActiveStyle
		}
		line := fmt.Sprintf("    %s %s", radioStyle.Render(radio), lipgloss.NewStyle().Foreground(theme.Text).Render(p))
		if i == m.cursor {
			line = fmt.Sprintf("    %s %s",
				lipgloss.NewStyle().Foreground(theme.BgDark).Bold(true).Render(radio),
				lipgloss.NewStyle().Foreground(theme.BgDark).Bold(true).Render(p),
			)
			line = lipgloss.NewStyle().Background(theme.Primary).Render(line)
		}
		providerRows = append(providerRows, line)
	}

	fieldsLabel := lipgloss.NewStyle().Foreground(theme.Secondary).Bold(true).Render("CONFIGURATION")
	fieldNames := []string{"MODEL", "API KEY", "BASE URL"}
	var fieldRows []string
	for fi := 0; fi < numFields; fi++ {
		rowIdx := len(llmProviders) + fi
		label := lipgloss.NewStyle().Width(10).Foreground(theme.TextDim).Render(fieldNames[fi])

		var value string
		if m.editing == fi {
			value = m.textInputs[fi].View()
		} else {
			raw := m.textInputs[fi].Value()
			if fi == fieldAPIKey && raw != "" {
				value = maskAPIKey(raw)
			} else if raw == "" {
				value = lipgloss.NewStyle().Foreground(theme.Muted).Render("(not set)")
			} else {
				value = lipgloss.NewStyle().Foreground(theme.Text).Render(raw)
			}
		}

		hint := ""
		if m.editing != fi {
			if fi == fieldModel {
				hint = lipgloss.NewStyle().Foreground(theme.Muted).Render("  [enter] select")
			} else {
				hint = lipgloss.NewStyle().Foreground(theme.Muted).Render("  [enter] edit")
			}
		}

		line := fmt.Sprintf("    %s  %s%s", label, value, hint)
		if rowIdx == m.cursor && m.editing < 0 {
			renderVal := m.textInputs[fi].Value()
			if fi == fieldAPIKey && renderVal != "" {
				renderVal = maskAPIKey(renderVal)
			}
			line = lipgloss.NewStyle().Background(theme.Primary).Foreground(theme.BgDark).Bold(true).
				Render(fmt.Sprintf("    %-10s  %s", fieldNames[fi], renderVal))
		}
		fieldRows = append(fieldRows, line)
	}

	hint := lipgloss.NewStyle().Foreground(theme.TextDim).Render("[j/k] navigate  [enter/space] select  [s] save  [esc] cancel")

	var lines []string
	lines = append(lines, title+subtitle)
	lines = append(lines, "")
	lines = append(lines, "  "+providerLabel)
	lines = append(lines, divider)
	lines = append(lines, providerRows...)
	lines = append(lines, "")
	lines = append(lines, "  "+fieldsLabel)
	lines = append(lines, divider)
	lines = append(lines, fieldRows...)
	if m.modelDropdown && len(m.modelOptions) > 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Secondary).Bold(true).Render("  MODELS"))
		lines = append(lines, divider)
		for i, model := range m.modelOptions {
			row := "    " + model
			if i == m.modelOptionCursor {
				row = lipgloss.NewStyle().Background(theme.Primary).Foreground(theme.BgDark).Bold(true).Render(row)
			}
			lines = append(lines, row)
		}
		lines = append(lines, "    "+lipgloss.NewStyle().Foreground(theme.TextDim).Render("[enter] choose  [esc] close"))
	}
	lines = append(lines, "")
	lines = append(lines, divider)
	lines = append(lines, hint)

	return lipgloss.NewStyle().Width(m.width).Height(m.height).Padding(1, 2).Render(strings.Join(lines, "\n"))
}

func maskAPIKey(key string) string {
	if len(key) <= 7 {
		return strings.Repeat("*", len(key))
	}
	return key[:3] + strings.Repeat("*", len(key)-7) + key[len(key)-4:]
}
