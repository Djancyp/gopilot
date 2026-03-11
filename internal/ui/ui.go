package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gopilot/gopilot/client"
)

// SetClientFunc is a function to get the AI client, set by main package
var SetClientFunc func() *Client

// Client alias for easier access
type Client = client.Client

// Model represents the main UI model
type Model struct {
	messages        []Message
	textInput       textinput.Model
	quitting        bool
	width           int
	height          int
	chatHeight      int
	isLoading       bool
	client          *Client
	status          Status
	currentResponse string
	reasoningText   string
	scrollOffset    int
	inputLines      []string
	cursorLine      int
	cursorPos       int
	inputScroll     int
}

// Status represents the client status
type Status int

const (
	StatusInitializing Status = iota
	StatusReady
	StatusError
)

// NewModel creates a new UI model
func NewModel(client *Client) Model {
	ti := textinput.New()
	ti.Placeholder = "Type your message..."
	ti.PromptStyle = InputPromptStyle
	ti.TextStyle = InputStyle
	ti.Focus()
	ti.CharLimit = 0
	ti.Width = 60

	status := StatusInitializing
	if client != nil {
		status = StatusReady
	}

	return Model{
		messages:   []Message{},
		textInput:  ti,
		chatHeight: 0,
		client:     client,
		status:     status,
		inputLines: []string{""},
		cursorLine: 0,
		cursorPos:  0,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width - 4
		// Calculate chat height: total height - title(3) - status(1) - input(3) - help(2) - margins(4)
		m.chatHeight = msg.Height - 13
		if m.chatHeight < 5 {
			m.chatHeight = 5
		}
		return m, nil

	case tea.MouseMsg:
		// Handle mouse wheel for scrolling
		totalLines := m.getTotalLines()
		
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case tea.MouseButtonWheelDown:
			maxScroll := totalLines - m.chatHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyPgUp:
			m.scrollOffset -= m.chatHeight / 2
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
			return m, nil

		case tea.KeyPgDown:
			m.scrollOffset += m.chatHeight / 2
			maxScroll := m.getTotalLines() - m.chatHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset > maxScroll {
				m.scrollOffset = maxScroll
			}
			return m, nil

		case tea.KeyCtrlJ:
			// Insert new line (Ctrl+J) - unlimited lines
			// Insert new line at cursor position
			currentLine := m.inputLines[m.cursorLine]
			m.inputLines = append(m.inputLines[:m.cursorLine+1], m.inputLines[m.cursorLine:]...)
			m.inputLines[m.cursorLine+1] = currentLine[m.cursorPos:]
			m.inputLines[m.cursorLine] = currentLine[:m.cursorPos]
			m.cursorLine++
			m.cursorPos = 0
			return m, nil

		case tea.KeyEnter:
			if m.status == StatusReady && !m.isLoading {
				// Join all lines
				userMsg := strings.Join(m.inputLines, "\n")
				if userMsg != "" {
					// Add user message
					m.messages = append(m.messages, NewMessage(UserMessage, userMsg))
					// Reset input
					m.inputLines = []string{""}
					m.cursorLine = 0
					m.cursorPos = 0
					// Start AI response
					m.isLoading = true
					m.currentResponse = ""
					return m, StartChat(userMsg)
				}
			}
			return m, nil

		case tea.KeyLeft:
			if m.cursorPos > 0 {
				m.cursorPos--
			} else if m.cursorLine > 0 {
				m.cursorLine--
				m.cursorPos = len(m.inputLines[m.cursorLine])
			}
			return m, nil

		case tea.KeyRight:
			if m.cursorPos < len(m.inputLines[m.cursorLine]) {
				m.cursorPos++
			} else if m.cursorLine < len(m.inputLines)-1 {
				m.cursorLine++
				m.cursorPos = 0
			}
			return m, nil

		case tea.KeyUp:
			if m.cursorLine > 0 {
				m.cursorLine--
				if m.cursorPos > len(m.inputLines[m.cursorLine]) {
					m.cursorPos = len(m.inputLines[m.cursorLine])
				}
			}
			return m, nil

		case tea.KeyDown:
			if m.cursorLine < len(m.inputLines)-1 {
				m.cursorLine++
				if m.cursorPos > len(m.inputLines[m.cursorLine]) {
					m.cursorPos = len(m.inputLines[m.cursorLine])
				}
			}
			return m, nil

		case tea.KeyBackspace:
			if m.cursorPos > 0 {
				m.inputLines[m.cursorLine] = m.inputLines[m.cursorLine][:m.cursorPos-1] + m.inputLines[m.cursorLine][m.cursorPos:]
				m.cursorPos--
			} else if m.cursorLine > 0 {
				// Merge with previous line
				m.inputLines[m.cursorLine-1] += m.inputLines[m.cursorLine]
				m.inputLines = append(m.inputLines[:m.cursorLine], m.inputLines[m.cursorLine+1:]...)
				m.cursorLine--
				m.cursorPos = len(m.inputLines[m.cursorLine])
			}
			return m, nil

		case tea.KeyDelete:
			if m.cursorPos < len(m.inputLines[m.cursorLine]) {
				m.inputLines[m.cursorLine] = m.inputLines[m.cursorLine][:m.cursorPos] + m.inputLines[m.cursorLine][m.cursorPos+1:]
			} else if m.cursorLine < len(m.inputLines)-1 {
				// Merge with next line
				m.inputLines[m.cursorLine] += m.inputLines[m.cursorLine+1]
				m.inputLines = append(m.inputLines[:m.cursorLine+1], m.inputLines[m.cursorLine+2:]...)
			}
			return m, nil

		default:
			// Handle regular character input
			if len(msg.Runes) == 1 {
				line := m.inputLines[m.cursorLine]
				m.inputLines[m.cursorLine] = line[:m.cursorPos] + string(msg.Runes[0]) + line[m.cursorPos:]
				m.cursorPos++
			}
			return m, nil
		}

	case StreamMsg:
		if msg.Error != nil {
			m.isLoading = false
			m.messages = append(m.messages, NewMessage(ErrorMessage, msg.Error.Error()))
			// Close channel on error
			if modelChan != nil {
				close(modelChan)
				modelChan = nil
			}
			return m, nil
		}

		if msg.IsComplete {
			m.isLoading = false
			m.currentResponse = ""
			m.reasoningText = ""
			m.status = StatusReady
			// Close channel on complete
			if modelChan != nil {
				close(modelChan)
				modelChan = nil
			}
			return m, nil
		}

		// Handle reasoning (thinking) text
		if msg.Reasoning != "" {
			m.reasoningText += msg.Reasoning
			// Find or create reasoning message
			found := -1
			for i := range m.messages {
				if m.messages[i].Type == AssistantMessage && strings.HasPrefix(m.messages[i].Content, "🧠 ") {
					found = i
					break
				}
			}
			if found >= 0 {
				m.messages[found].Content = "🧠 " + m.reasoningText
			} else {
				m.messages = append(m.messages, NewMessage(AssistantMessage, "🧠 "+m.reasoningText))
			}
			// Continue streaming
			return m, ContinueChat()
		}

		// Handle regular content
		if msg.Content != "" {
			m.currentResponse += msg.Content

			// Remove any existing reasoning message
			cleanedMessages := []Message{}
			for _, msg := range m.messages {
				if msg.Type != AssistantMessage || !strings.HasPrefix(msg.Content, "🧠 ") {
					cleanedMessages = append(cleanedMessages, msg)
				}
			}
			m.messages = cleanedMessages

			// Always update the last assistant message (which is the current response)
			// If there's no assistant message yet, create one
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].Type == AssistantMessage {
				m.messages[len(m.messages)-1].Content = m.currentResponse
			} else {
				m.messages = append(m.messages, NewMessage(AssistantMessage, m.currentResponse))
			}
			
			// Auto-scroll to bottom
			m.scrollOffset = m.getTotalLines() - m.chatHeight
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
			
			// Continue streaming
			return m, ContinueChat()
		}

	case InitClientMsg:
		if msg.Err != nil {
			m.status = StatusError
			m.messages = append(m.messages, NewMessage(ErrorMessage, "Failed to initialize AI: "+msg.Err.Error()))
		} else {
			m.status = StatusReady
			if SetClientFunc != nil {
				m.client = SetClientFunc()
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View renders the UI
func (m Model) View() string {
	var b strings.Builder

	// Status indicator
	b.WriteString(m.renderStatus())
	b.WriteString("\n\n")

	// Chat area
	chatView := m.renderChat()
	b.WriteString(ChatContainerStyle.
		Width(m.width - 4).
		Height(m.chatHeight).
		Render(chatView))

	// Spacer to push input to bottom
	remainingHeight := m.height - 13 - m.chatHeight
	for i := 0; i < remainingHeight && i < 2; i++ {
		b.WriteString("\n")
	}

	// Input area at bottom with border
	b.WriteString("\n")
	
	// Build input content
	var inputContent strings.Builder
	visibleLines := 3
	if len(m.inputLines) < visibleLines {
		visibleLines = len(m.inputLines)
	}
	
	// Calculate input scroll to keep cursor visible
	if m.cursorLine >= m.inputScroll + visibleLines {
		m.inputScroll = m.cursorLine - visibleLines + 1
	}
	if m.cursorLine < m.inputScroll {
		m.inputScroll = m.cursorLine
	}
	
	// Render visible lines
	for i := m.inputScroll; i < m.inputScroll+visibleLines && i < len(m.inputLines); i++ {
		line := m.inputLines[i]
		if i == m.cursorLine {
			// Show cursor on current line
			cursor := InputPromptStyle.Render("> ")
			if m.cursorPos <= len(line) {
				inputContent.WriteString(cursor + line[:m.cursorPos] + "█" + line[m.cursorPos:])
			} else {
				inputContent.WriteString(cursor + line + "█")
			}
		} else {
			inputContent.WriteString("  " + line)
		}
		if i < len(m.inputLines)-1 && i < m.inputScroll+visibleLines-1 {
			inputContent.WriteString("\n")
		}
	}
	
	// Wrap with border
	if m.isLoading {
		b.WriteString(LoadingStyle.Render("⏳ Thinking..."))
		b.WriteString("\n")
	}
	
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#333333")).
		Padding(0, 1).
		Width(m.width - 4)
	
	switch m.status {
	case StatusReady:
		b.WriteString(inputBox.Render(inputContent.String()))
	case StatusInitializing:
		b.WriteString(inputBox.Render(LoadingStyle.Render("⏳ Loading AI model...")))
	case StatusError:
		b.WriteString(inputBox.Render(ErrorMessageStyle.Render("❌ AI not available")))
	}

	// Help text at bottom
	b.WriteString("\n")
	b.WriteString(HelpStyle.Render("Ctrl+C: Quit | Enter: Send | Ctrl+J: New line | Mouse/PgUp/PgDn: Scroll | ↑↓←→: Navigate input"))

	// Quit message
	if m.quitting {
		b.WriteString("\n\n")
		b.WriteString(SubtitleStyle.Render("Goodbye! 👋"))
	}

	return b.String()
}

// renderStatus renders the status indicator
func (m Model) renderStatus() string {
	var status string
	switch m.status {
	case StatusReady:
		status = ReadyStyle.Render("● Ready")
	case StatusInitializing:
		status = LoadingStyle.Render("● Loading...")
	case StatusError:
		status = ErrorMessageStyle.Render("● Error")
	default:
		status = "● Unknown"
	}

	// Add model info
	if m.client != nil && m.status == StatusReady {
		modelInfo := SubtitleStyle.Render("Model: " + m.client.GetModelName())
		status += "  " + modelInfo
	}

	// Add loading indicator
	if m.isLoading {
		status += " " + LoadingStyle.Render("| Loading...")
	}

	return status
}

// renderChat renders the chat messages
func (m Model) renderChat() string {
	if len(m.messages) == 0 {
		return lipgloss.NewStyle().
			Foreground(MutedColor).
			Italic(true).
			Render("Welcome to GoPilot! Start a conversation...")
	}

	var messages []string
	var totalLines int
	for _, msg := range m.messages {
		rendered := m.renderMessage(msg)
		messages = append(messages, rendered)
		// Count lines in rendered message
		totalLines += len(strings.Split(rendered, "\n"))
	}

	// Calculate visible range based on actual line count
	visibleLines := m.chatHeight
	
	// Ensure scrollOffset is within bounds
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	maxScroll := totalLines - visibleLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	
	// Build visible portion
	var visibleMessages []string
	var currentLine int
	for _, msg := range messages {
		msgLines := strings.Split(msg, "\n")
		for _, line := range msgLines {
			if currentLine >= m.scrollOffset && currentLine < m.scrollOffset+visibleLines {
				visibleMessages = append(visibleMessages, line)
			}
			currentLine++
		}
	}

	return strings.Join(visibleMessages, "\n")
}

// renderMessage renders a single message
func (m Model) renderMessage(msg Message) string {
	switch msg.Type {
	case UserMessage:
		return UserMessageStyle.Width(m.width - 10).Render(msg.Content)
	case AssistantMessage:
		// Check if this is a reasoning message
		if strings.HasPrefix(msg.Content, "🧠 ") {
			return ReasoningMessageStyle.Render(strings.TrimPrefix(msg.Content, "🧠 "))
		}
		return AssistantMessageStyle.Width(m.width - 12).Render(msg.Content)
	case SystemMessage:
		return SystemMessageStyle.Render("ℹ️ " + msg.Content)
	case ErrorMessage:
		return ErrorMessageStyle.Render("❌ Error: " + msg.Content)
	default:
		return msg.Content
	}
}

// StreamMsg represents a streaming response from the AI
type StreamMsg struct {
	Content    string
	Reasoning  string
	IsComplete bool
	Error      error
}

// InitClientMsg represents client initialization result
type InitClientMsg struct {
	Err error
}

// modelRef holds a reference to send messages back
var modelChan chan tea.Msg
var modelCancel func()

// StartChat starts a chat request
func StartChat(message string) tea.Cmd {
	// Cancel any existing chat
	if modelCancel != nil {
		modelCancel()
	}
	if modelChan != nil {
		// Drain the channel
		for range modelChan {
		}
	}
	
	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	modelCancel = cancel
	
	// Create channel for this chat session
	modelChan = make(chan tea.Msg, 100)

	// Start goroutine to process streaming response
	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case modelChan <- StreamMsg{Error: fmt.Errorf("panic: %v", r)}:
				default:
				}
			}
			// Don't close here, let ContinueChat handle it
		}()

		c := getAIClient()
		if c == nil {
			select {
			case modelChan <- StreamMsg{
				Error:      nil,
				IsComplete: true,
				Content:    "AI client not initialized. Running in demo mode.",
			}:
			default:
			}
			return
		}

		ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		ch, err := c.Chat(ctx, message)
		if err != nil {
			select {
			case modelChan <- StreamMsg{Error: fmt.Errorf("chat error: %w", err)}:
			default:
			}
			return
		}

		for resp := range ch {
			select {
			case <-ctx.Done():
				return
			default:
				if resp.Err != nil {
					select {
					case modelChan <- StreamMsg{Error: resp.Err}:
					default:
					}
					return
				}
				if resp.IsComplete {
					select {
					case modelChan <- StreamMsg{IsComplete: true}:
					default:
					}
					return
				}
				if resp.Reasoning != "" {
					select {
					case modelChan <- StreamMsg{Reasoning: resp.Reasoning}:
					default:
					}
				}
				if resp.Content != "" {
					select {
					case modelChan <- StreamMsg{Content: resp.Content}:
					default:
					}
				}
			}
		}
	}()

	// Return command that reads from channel
	return func() tea.Msg {
		msg, ok := <-modelChan
		if !ok {
			return StreamMsg{IsComplete: true}
		}
		return msg
	}
}

// ContinueChat continues reading from the chat stream
func ContinueChat() tea.Cmd {
	if modelChan == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-modelChan
		if !ok {
			modelChan = nil
			modelCancel = nil
			return StreamMsg{IsComplete: true}
		}
		return msg
	}
}

// getAIClient returns the global AI client from main package
func getAIClient() *Client {
	if SetClientFunc != nil {
		return SetClientFunc()
	}
	return nil
}

// getTotalLines calculates total display lines in chat
func (m Model) getTotalLines() int {
	total := 0
	for _, msg := range m.messages {
		rendered := m.renderMessage(msg)
		total += len(strings.Split(rendered, "\n"))
	}
	return total
}
