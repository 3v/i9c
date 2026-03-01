package views

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"i9c/internal/tui/theme"
)

type FolderSelectedMsg struct {
	Path string
}

type FolderCancelMsg struct{}

type dirEntry struct {
	name  string
	isDir bool
}

type FolderPickerModel struct {
	width, height int
	input         textinput.Model
	browseDir     string
	entries       []dirEntry
	cursor        int
	inputFocused  bool
	err           string
}

func NewFolderPickerModel(startDir string) *FolderPickerModel {
	ti := textinput.New()
	ti.Prompt = "Path: "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(theme.Secondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Primary)
	ti.Placeholder = "/path/to/iac/folder"
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	ti.CharLimit = 500
	ti.Width = 60

	absDir, err := filepath.Abs(startDir)
	if err != nil {
		absDir = startDir
	}
	ti.SetValue(absDir)

	m := &FolderPickerModel{
		input:        ti,
		browseDir:    absDir,
		inputFocused: false,
	}
	m.loadDir(absDir)
	return m
}

func (m *FolderPickerModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.input.Width = max(30, w-12)
}

func (m *FolderPickerModel) loadDir(dir string) {
	m.entries = nil
	m.err = ""

	info, err := os.Stat(dir)
	if err != nil {
		m.err = "Directory not found: " + dir
		return
	}
	if !info.IsDir() {
		m.err = "Not a directory: " + dir
		return
	}

	osEntries, err := os.ReadDir(dir)
	if err != nil {
		m.err = "Cannot read directory: " + err.Error()
		return
	}

	if dir != "/" {
		m.entries = append(m.entries, dirEntry{name: "..", isDir: true})
	}

	var dirs, files []dirEntry
	for _, e := range osEntries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, dirEntry{name: e.Name(), isDir: true})
		} else {
			files = append(files, dirEntry{name: e.Name(), isDir: false})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	m.entries = append(m.entries, dirs...)
	m.entries = append(m.entries, files...)

	m.browseDir = dir
	m.cursor = 0
}

func (m *FolderPickerModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.inputFocused {
			return m.handleInputKey(msg)
		}
		return m.handleBrowserKey(msg)
	}
	return nil
}

func (m *FolderPickerModel) handleInputKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		return func() tea.Msg { return FolderCancelMsg{} }
	case tea.KeyTab:
		m.inputFocused = false
		m.input.Blur()
		path := strings.TrimSpace(m.input.Value())
		if path != "" {
			m.loadDir(path)
		}
		return nil
	case tea.KeyEnter:
		path := strings.TrimSpace(m.input.Value())
		if path == "" {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			m.err = "Path not found: " + path
			return nil
		}
		if !info.IsDir() {
			m.err = "Not a directory: " + path
			return nil
		}
		return func() tea.Msg { return FolderSelectedMsg{Path: path} }
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

func (m *FolderPickerModel) handleBrowserKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		return func() tea.Msg { return FolderCancelMsg{} }
	case "tab":
		m.inputFocused = true
		return m.input.Focus()
	case "j", "down":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter", "o", "right":
		if m.cursor < len(m.entries) {
			e := m.entries[m.cursor]
			if e.isDir {
				var target string
				if e.name == ".." {
					target = filepath.Dir(m.browseDir)
				} else {
					target = filepath.Join(m.browseDir, e.name)
				}
				m.loadDir(target)
				m.input.SetValue(m.browseDir)
			}
		}
	case "s":
		return func() tea.Msg { return FolderSelectedMsg{Path: m.browseDir} }
	case "backspace", "h":
		parent := filepath.Dir(m.browseDir)
		if parent != m.browseDir {
			m.loadDir(parent)
			m.input.SetValue(m.browseDir)
		}
	case "G":
		m.cursor = max(0, len(m.entries)-1)
	case "g":
		m.cursor = 0
	}
	return nil
}

func (m *FolderPickerModel) View() string {
	title := theme.TitleStyle.Render("Select IaC Folder")
	subtitle := lipgloss.NewStyle().Foreground(theme.TextDim).Render(
		"  Choose the directory to monitor for Terraform/OpenTofu files")

	var inputView string
	if m.inputFocused {
		inputView = m.input.View()
	} else {
		inputView = lipgloss.NewStyle().Foreground(theme.TextDim).Render("Path: ") +
			lipgloss.NewStyle().Foreground(theme.Text).Render(m.input.Value())
	}

	modeLine := ""
	if m.inputFocused {
		modeLine = lipgloss.NewStyle().Foreground(theme.Secondary).Render("[input mode]") +
			lipgloss.NewStyle().Foreground(theme.TextDim).Render("  [tab] switch to browser  [enter] select  [esc] cancel")
	} else {
		modeLine = lipgloss.NewStyle().Foreground(theme.Primary).Render("[browser mode]") +
			lipgloss.NewStyle().Foreground(theme.TextDim).Render("  [enter] open highlighted dir  [s] select current dir  [tab] switch to input  [esc] cancel")
	}

	currentDir := theme.HelpKeyStyle.Render("Browsing: ") +
		lipgloss.NewStyle().Foreground(theme.Text).Bold(true).Render(m.browseDir)

	separator := lipgloss.NewStyle().
		Foreground(theme.Border).
		Render(strings.Repeat("─", min(m.width-6, 80)))

	if m.err != "" {
		errLine := lipgloss.NewStyle().Foreground(theme.Danger).Render(m.err)
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Padding(1, 2).
			Render(strings.Join([]string{
				title + subtitle, "", inputView, modeLine, "", currentDir, separator, errLine,
			}, "\n"))
	}

	visibleRows := m.height - 10
	if visibleRows < 1 {
		visibleRows = 1
	}

	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}
	end := start + visibleRows
	if end > len(m.entries) {
		end = len(m.entries)
	}

	var rows []string
	for i := start; i < end; i++ {
		e := m.entries[i]
		var line string
		if e.isDir {
			name := e.name + "/"
			if i == m.cursor && !m.inputFocused {
				line = lipgloss.NewStyle().
					Background(theme.Primary).
					Foreground(theme.BgDark).
					Bold(true).
					Render("  > " + name)
			} else {
				line = lipgloss.NewStyle().Foreground(theme.Secondary).Render("    " + name)
			}
		} else {
			if i == m.cursor && !m.inputFocused {
				line = lipgloss.NewStyle().
					Background(theme.Primary).
					Foreground(theme.BgDark).
					Bold(true).
					Render("  > " + e.name)
			} else {
				line = lipgloss.NewStyle().Foreground(theme.TextDim).Render("    " + e.name)
			}
		}
		rows = append(rows, line)
	}

	browser := strings.Join(rows, "\n")

	hint := lipgloss.NewStyle().Foreground(theme.TextDim).Render(
		"[j/k] navigate  [enter] open dir  [backspace] up  [s] select dir  [tab] toggle mode  [esc] cancel")

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Padding(1, 2).
		Render(strings.Join([]string{
			title + subtitle, "", inputView, modeLine, "", currentDir, separator, browser, "", hint,
		}, "\n"))
}
