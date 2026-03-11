package modal

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#FFA500")).
			Bold(true).
			Padding(0, 1)

	unselectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#808080")).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500")).
			Bold(true)

	commandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFA500")).
			Background(lipgloss.Color("#1a1a1a")).
			Padding(1, 2)
)

// Option represents a selectable option in the modal
type Option struct {
	Label       string
	Description string
	Key         string
}

// Model represents the execution permission modal
type Model struct {
	options     []Option
	cursor      int
	command     string
	width       int
	height      int
	quitting    bool
	selected    int // -1 for none, 0+ for option index
}

// KeyMap defines key bindings
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Select   key.Binding
	Cancel   key.Binding
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter", " "),
			key.WithHelp("enter", "select"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "ctrl+c"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

// New creates a new execution modal
func New(command string) Model {
	options := []Option{
		{
			Label:       "Execute",
			Description: "Run this command once",
			Key:         "enter",
		},
		{
			Label:       "Auto-execute (session)",
			Description: "Run commands without asking for this session",
			Key:         "tab",
		},
		{
			Label:       "Cancel",
			Description: "Dismiss without executing",
			Key:         "esc",
		},
	}

	return Model{
		options: options,
		cursor:  0,
		command: command,
		selected: -1,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.selected = -1 // Cancel
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = len(m.options) - 1
			}

		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}

		case "enter", " ":
			m.selected = m.cursor
			m.quitting = true
			return m, tea.Quit

		case "tab":
			m.selected = 1 // Auto-execute option
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View renders the modal
func (m Model) View() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("⚠ Execute Command?"))
	b.WriteString("\n\n")

	// Command
	b.WriteString(commandStyle.Render("Command: " + m.command))
	b.WriteString("\n\n")

	// Options
	for i, opt := range m.options {
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("❯ " + opt.Label))
		} else {
			b.WriteString(unselectedStyle.Render("  " + opt.Label))
		}
		b.WriteString(" - " + opt.Description)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(unselectedStyle.Render("↑↓ Navigate  |  Enter Select  |  Esc Cancel"))

	return borderStyle.Render(b.String())
}

// GetSelected returns the selected option index
func (m Model) GetSelected() int {
	return m.selected
}

// GetCommand returns the command string
func (m Model) GetCommand() string {
	return m.command
}

// IsQuitting returns true if modal should close
func (m Model) IsQuitting() bool {
	return m.quitting
}

// Cursor returns the current cursor position
func (m *Model) Cursor() int {
	return m.cursor
}

// CursorUp moves the cursor up
func (m *Model) CursorUp() {
	if m.cursor > 0 {
		m.cursor--
	} else {
		m.cursor = len(m.options) - 1
	}
}

// CursorDown moves the cursor down
func (m *Model) CursorDown() {
	if m.cursor < len(m.options)-1 {
		m.cursor++
	} else {
		m.cursor = 0
	}
}
