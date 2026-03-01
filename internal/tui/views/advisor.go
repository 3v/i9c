package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/config"
	"i9c/internal/tui/theme"
)

type ChatMessage struct {
	Role    string
	Content string
}

type AdvisorModel struct {
	cfg           *config.Config
	width, height int
	messages      []ChatMessage
	input         textarea.Model
	chatRatio     float64
	scrollOffset  int
	streaming     bool
	busy          bool
	activityText  string
	activitySince time.Time
	activityLive  bool
}

func NewAdvisorModel(cfg *config.Config) *AdvisorModel {
	ti := textarea.New()
	ti.Prompt = "> "
	ti.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(theme.Secondary).Bold(true)
	ti.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(theme.Secondary).Bold(true)
	ti.Placeholder = "Ask about your infrastructure..."
	ti.CharLimit = 8000
	ti.SetWidth(76)
	ti.SetHeight(4)
	ti.ShowLineNumbers = false
	ti.Focus()

	return &AdvisorModel{
		cfg:       cfg,
		input:     ti,
		chatRatio: 0.60,
		messages: []ChatMessage{
			{
				Role: "assistant",
				Content: "Welcome to i9c Advisor. I can help you understand your infrastructure, explain drift, and generate Terraform/OpenTofu code.\n\n" +
					"Try asking:\n" +
					"  - \"Explain the current drift\"\n" +
					"  - \"Generate a VPC with 3 AZs\"\n" +
					"  - \"Fix the security group issue\"",
			},
		},
	}
}

func (m *AdvisorModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	inputPane := m.inputPaneHeight()
	inputEditorHeight := max(2, inputPane-5)
	m.input.SetWidth(max(20, w-14))
	m.input.SetHeight(inputEditorHeight)
}

func (m *AdvisorModel) IsInputActive() bool {
	return m.input.Focused()
}

func (m *AdvisorModel) ComposerValue() string {
	return m.input.Value()
}

type AdvisorResponseMsg struct {
	Content string
	Done    bool
}

type AdvisorSendMsg struct {
	Text string
}

type AdvisorBusyMsg struct {
	Busy bool
}

type AdvisorCancelMsg struct{}
type AdvisorActivityMsg struct {
	Text    string
	Running bool
}
type advisorActivityTickMsg struct{}

func (m *AdvisorModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case AdvisorBusyMsg:
		m.busy = msg.Busy
		if m.busy {
			m.input.Blur()
		}
		return nil
	case AdvisorActivityMsg:
		text := strings.TrimSpace(msg.Text)
		if text != "" {
			m.activityText = text
			m.activitySince = time.Now()
		}
		m.activityLive = msg.Running
		if m.activityLive {
			return advisorActivityTickCmd()
		}
		return nil
	case AdvisorResponseMsg:
		if m.streaming && len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
			m.messages[len(m.messages)-1].Content += msg.Content
		} else {
			m.messages = append(m.messages, ChatMessage{Role: "assistant", Content: msg.Content})
			m.streaming = true
		}
		if msg.Done {
			m.streaming = false
		}
		return nil
	case advisorActivityTickMsg:
		if !m.activityLive {
			return nil
		}
		return advisorActivityTickCmd()

	case tea.KeyMsg:
		if m.busy {
			switch msg.String() {
			case "x":
				return func() tea.Msg { return AdvisorCancelMsg{} }
			case "j", "down":
				m.scrollOffset = max(0, m.scrollOffset-1)
			case "k", "up":
				m.scrollOffset = min(m.maxScroll(), m.scrollOffset+1)
			}
			return nil
		}
		if m.input.Focused() {
			switch msg.String() {
			case "esc":
				m.input.Blur()
				return nil
			case "enter":
				text := strings.TrimSpace(m.input.Value())
				if text == "" {
					return nil
				}
				m.messages = append(m.messages, ChatMessage{Role: "user", Content: text})
				m.input.Reset()
				m.input.SetCursor(0)
				m.scrollOffset = 0
				return func() tea.Msg {
					return AdvisorSendMsg{Text: text}
				}
			case "shift+enter":
				m.input.InsertRune('\n')
				return nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return cmd
		}

		chatHeight := m.chatHeight()
		maxScroll := m.maxScroll()

		switch msg.String() {
		case "i":
			if !m.busy {
				return m.input.Focus()
			}
			return nil
		case "j", "down":
			m.scrollOffset = max(0, m.scrollOffset-1)
		case "k", "up":
			m.scrollOffset = min(maxScroll, m.scrollOffset+1)
		case "G":
			m.scrollOffset = 0
		case "g":
			m.scrollOffset = maxScroll
		case "ctrl+d":
			m.scrollOffset = max(0, m.scrollOffset-chatHeight/2)
		case "ctrl+u":
			m.scrollOffset = min(maxScroll, m.scrollOffset+chatHeight/2)
		case "pgdown":
			m.scrollOffset = max(0, m.scrollOffset-chatHeight/2)
		case "pgup":
			m.scrollOffset = min(maxScroll, m.scrollOffset+chatHeight/2)
		}
	}
	return nil
}

func (m *AdvisorModel) chatHeight() int {
	// pane interior = paneHeight - 2 borders; reserve 1 interior row for "Responses" title.
	h := m.chatPaneHeight() - 3
	if h < 1 {
		h = 1
	}
	return h
}

func (m *AdvisorModel) paneBudget() int {
	// Reserve room for title/subtitle and outer spacing.
	budget := m.height - 7
	if budget < 8 {
		budget = 8
	}
	return budget
}

func (m *AdvisorModel) chatPaneHeight() int {
	budget := m.paneBudget()
	chat := int(float64(budget) * m.chatRatio)
	if chat < 6 {
		chat = 6
	}
	if chat > budget-4 {
		chat = budget - 4
	}
	return chat
}

func (m *AdvisorModel) inputPaneHeight() int {
	h := m.paneBudget() - m.chatPaneHeight()
	if h < 4 {
		h = 4
	}
	return h
}

func (m *AdvisorModel) renderChatLines() []string {
	var rendered []string
	maxWidth := max(20, m.width-18)
	for _, msg := range m.messages {
		var prefix string
		var style lipgloss.Style
		if msg.Role == "user" {
			prefix = theme.ChatUserStyle.Render("You: ")
			style = lipgloss.NewStyle().Foreground(theme.Text)
		} else {
			prefix = theme.ChatAssistantStyle.Render("i9c: ")
			style = lipgloss.NewStyle().Foreground(theme.Text)
		}

		lines := strings.Split(msg.Content, "\n")
		for i, line := range lines {
			wrappedLines := wrapText(line, maxWidth)
			if len(wrappedLines) == 0 {
				wrappedLines = []string{""}
			}
			if i == 0 {
				rendered = append(rendered, prefix+style.Render(wrappedLines[0]))
				for _, wl := range wrappedLines[1:] {
					rendered = append(rendered, "      "+style.Render(wl))
				}
			} else {
				for _, wl := range wrappedLines {
					rendered = append(rendered, "      "+style.Render(wl))
				}
			}
		}
		rendered = append(rendered, "")
	}

	if m.streaming {
		rendered = append(rendered, theme.ChatAssistantStyle.Render("i9c: ")+"...")
	}

	return rendered
}

func wrapText(s string, width int) []string {
	if width <= 1 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	cur := ""
	for _, w := range words {
		// hard-split very long tokens
		for len([]rune(w)) > width {
			chunk := string([]rune(w)[:width-1]) + "…"
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			lines = append(lines, chunk)
			w = string([]rune(w)[width-1:])
		}
		if cur == "" {
			cur = w
			continue
		}
		next := cur + " " + w
		if len([]rune(next)) > width {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = next
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func (m *AdvisorModel) maxScroll() int {
	totalLines := len(m.renderChatLines())
	ms := totalLines - m.chatHeight()
	if ms < 0 {
		return 0
	}
	return ms
}

func (m *AdvisorModel) View() string {
	title := theme.TitleStyle.Render("AI Advisor")

	providerInfo := lipgloss.NewStyle().Foreground(theme.TextDim).Render(
		"  Provider: " + m.cfg.LLM.Provider + " / " + m.cfg.LLM.Model)
	activityInfo := lipgloss.NewStyle().Foreground(theme.TextDim).Render("  Activity: " + m.activityLine())

	chatHeight := m.chatHeight()
	chatLines := m.renderChatLines()

	maxScroll := m.maxScroll()
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	if len(chatLines) > chatHeight {
		start := len(chatLines) - chatHeight - m.scrollOffset
		if start < 0 {
			start = 0
		}
		end := start + chatHeight
		if end > len(chatLines) {
			end = len(chatLines)
		}
		chatLines = chatLines[start:end]
	}
	chat := strings.Join(chatLines, "\n")
	chatPane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(max(20, m.width-8)).
		Height(m.chatPaneHeight()).
		Render("Responses\n" + chat)

	var inputLine string
	var inputHint string
	if m.busy {
		inputLine = lipgloss.NewStyle().Foreground(theme.Warning).Render("Assistant running... composer locked. Press [x] to cancel.")
		inputHint = lipgloss.NewStyle().Foreground(theme.TextDim).Render("[x] cancel  [j/k] scroll responses")
	} else if m.input.Focused() {
		inputLine = m.input.View()
		inputHint = lipgloss.NewStyle().Foreground(theme.TextDim).Render("[enter] send  [shift+enter] newline  [esc] blur  [j/k] scroll responses")
	} else {
		inputLine = lipgloss.NewStyle().Foreground(theme.TextDim).Render("Composer ready. Press [i] to focus.")
		inputHint = lipgloss.NewStyle().Foreground(theme.TextDim).Render("[i] focus composer  [j/k] scroll responses")
	}
	inputPane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(max(20, m.width-8)).
		Height(m.inputPaneHeight()).
		Render("Composer\n" + inputLine + "\n" + inputHint)

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Padding(1, 2).
		Render(strings.Join([]string{title + providerInfo, activityInfo, "", chatPane, "", inputPane}, "\n"))
}

func advisorActivityTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return advisorActivityTickMsg{}
	})
}

func (m *AdvisorModel) activityLine() string {
	text := m.activityText
	if text == "" {
		if m.busy {
			text = "processing..."
		} else {
			return "idle"
		}
	}
	if m.activitySince.IsZero() {
		return text
	}
	elapsed := int(time.Since(m.activitySince).Seconds())
	if elapsed < 0 {
		elapsed = 0
	}
	return fmt.Sprintf("%s (%ds)", text, elapsed)
}
