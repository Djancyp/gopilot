package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gopilot/gopilot/client"
	"github.com/gopilot/gopilot/internal/modal"
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
	copyStatus      string
	copyStatusTimer int
	lastCommand     string
	allCommands     []string
	execStatus      string
	execStatusTimer int
	execModal       *modal.Model
	autoExecute     bool
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
	return tea.Batch(textinput.Blink, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{}
	}))
}

// tickMsg is a message type for timer ticks
type tickMsg struct{}

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
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
			return tickMsg{}
		})

	case tickMsg:
		// Decrement copy status timer
		if m.copyStatusTimer > 0 {
			m.copyStatusTimer--
			if m.copyStatusTimer == 0 {
				m.copyStatus = ""
			}
		}
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
			return tickMsg{}
		})

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

		case tea.KeyCtrlK:
			// Copy last message to clipboard
			if len(m.messages) > 0 {
				lastMsg := m.messages[len(m.messages)-1]
				if err := clipboard.WriteAll(lastMsg.Content); err == nil {
					m.copyStatus = "✓ Copied!"
					m.copyStatusTimer = 20
				} else {
					m.copyStatus = "✗ Copy failed"
					m.copyStatusTimer = 20
				}
			}
			return m, nil

		case tea.KeyEsc:
			if m.execModal != nil {
				m.execModal = nil
				m.lastCommand = ""
			}
			return m, nil

		case tea.KeyTab:
			if m.execModal != nil {
				// Toggle auto-execute for session
				m.autoExecute = !m.autoExecute
			}
			return m, nil

		case tea.KeyEnter:
			if m.execModal != nil {
				// Execute command based on cursor position
				// Cursor 0 = Execute all, 1 = Auto-execute (session), 2 = Cancel
				if m.execModal.Cursor() == 0 {
					// Execute all commands
					if len(m.allCommands) > 0 && m.status == StatusReady {
						m.isLoading = true
						m.execModal = nil
						m.execStatus = "Executing commands..."
						return m, ExecuteCommands(m.allCommands)
					}
				} else if m.execModal.Cursor() == 1 {
					// Auto-execute for session
					m.autoExecute = true
					if len(m.allCommands) > 0 && m.status == StatusReady {
						m.isLoading = true
						m.execModal = nil
						m.execStatus = "Executing commands..."
						return m, ExecuteCommands(m.allCommands)
					}
				} else {
					// Cancel
					m.execModal = nil
					m.lastCommand = ""
					m.allCommands = nil
				}
			}
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

		case tea.KeyUp:
			if m.execModal != nil {
				m.execModal.CursorUp()
			} else if m.cursorLine > 0 {
				m.cursorLine--
				if m.cursorPos > len(m.inputLines[m.cursorLine]) {
					m.cursorPos = len(m.inputLines[m.cursorLine])
				}
			}
			return m, nil

		case tea.KeyDown:
			if m.execModal != nil {
				m.execModal.CursorDown()
			} else if m.cursorLine < len(m.inputLines)-1 {
				m.cursorLine++
				if m.cursorPos > len(m.inputLines[m.cursorLine]) {
					m.cursorPos = len(m.inputLines[m.cursorLine])
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
			currentLine := m.inputLines[m.cursorLine]
			m.inputLines = append(m.inputLines[:m.cursorLine+1], m.inputLines[m.cursorLine:]...)
			m.inputLines[m.cursorLine+1] = currentLine[m.cursorPos:]
			m.inputLines[m.cursorLine] = currentLine[:m.cursorPos]
			m.cursorLine++
			m.cursorPos = 0
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
			
			// Check for COMMAND: in the last assistant message and execute all
			if m.execModal == nil && len(m.messages) > 0 {
				lastMsg := m.messages[len(m.messages)-1]
				if lastMsg.Type == AssistantMessage {
					// Extract ALL commands from the response
					commands := extractCommands(lastMsg.Content)
					if len(commands) > 0 {
						// Validate commands - skip if they look like natural language
						validCommands := filterValidCommands(commands)
						if len(validCommands) > 0 {
							if m.autoExecute {
								// Auto-execute all commands for this session
								m.isLoading = true
								m.execStatus = "Executing commands..."
								return m, ExecuteCommands(validCommands)
							} else {
								// Show modal with first command (user can enable auto-execute)
								m.lastCommand = validCommands[0]
								m.allCommands = validCommands
								modalModel := modal.New(validCommands[0])
								m.execModal = &modalModel
							}
						}
						// If no valid commands, just show the AI response (it's probably explanatory)
					}
				}
			}
			
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

	case ExecResultMsg:
		m.isLoading = false
		if msg.Err != nil {
			m.execStatus = "✗ Command failed"
			m.messages = append(m.messages, NewMessage(ErrorMessage, "Command failed: "+msg.Err.Error()))
		} else {
			m.execStatus = "✓ Command executed"
			m.messages = append(m.messages, NewMessage(SystemMessage, "Command output:\n```\n"+msg.Output+"```"))
		}
		m.execStatusTimer = 30
		m.lastCommand = ""
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
	helpText := "Ctrl+C: Quit | Enter: Send | Tab: Toggle auto | Esc: Cancel | Ctrl+K: Copy"
	if m.execStatus != "" {
		helpText = m.execStatus + "  " + helpText
	} else if m.copyStatus != "" {
		helpText = m.copyStatus + "  " + helpText
	}
	b.WriteString(HelpStyle.Render(helpText))

	// Quit message
	if m.quitting {
		b.WriteString("\n\n")
		b.WriteString(SubtitleStyle.Render("Goodbye! 👋"))
	}

	// Show execution permission modal as overlay on top of chat
	if m.execModal != nil {
		modalView := m.execModal.View()
		
		// Calculate modal dimensions
		modalWidth := lipgloss.Width(modalView)
		modalHeight := strings.Count(modalView, "\n") + 1
		
		// Center horizontally
		x := (m.width - modalWidth) / 2
		if x < 0 {
			x = 0
		}
		
		// Center vertically
		y := (m.height - modalHeight) / 2
		if y < 0 {
			y = 0
		}
		
		// Build overlay by splitting chat into lines and inserting modal
		lines := strings.Split(b.String(), "\n")
		
		// Ensure we have enough lines
		for len(lines) < m.height {
			lines = append(lines, "")
		}
		
		// Overlay modal lines
		modalLines := strings.Split(modalView, "\n")
		for i, modalLine := range modalLines {
			lineIdx := y + i
			if lineIdx >= 0 && lineIdx < len(lines) {
				// Pad the existing line if needed
				existingLine := lines[lineIdx]
				for len(existingLine) < x {
					existingLine += " "
				}
				// Overlay modal line (truncate if too long)
				if x+len(modalLine) > len(existingLine) {
					existingLine = existingLine[:x] + modalLine
				} else {
					existingLine = existingLine[:x] + modalLine + existingLine[x+len(modalLine):]
				}
				lines[lineIdx] = existingLine
			}
		}
		
		return strings.Join(lines, "\n")
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

	// Add scrollbar on the right if needed
	if totalLines > visibleLines {
		return m.renderWithScrollbar(visibleMessages, totalLines, visibleLines)
	}

	return strings.Join(visibleMessages, "\n")
}

// renderWithScrollbar renders chat with a scrollbar on the right
func (m Model) renderWithScrollbar(lines []string, totalLines, visibleLines int) string {
	// Calculate thumb position and size
	thumbSize := visibleLines * visibleLines / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}
	
	thumbPos := 0
	if totalLines > visibleLines {
		thumbPos = m.scrollOffset * visibleLines / totalLines
	}
	
	var result strings.Builder
	for i, line := range lines {
		result.WriteString(line)
		// Add scrollbar character
		if i >= thumbPos && i < thumbPos+thumbSize {
			result.WriteString(lipgloss.NewStyle().Foreground(ScrollbarColor).Render("│"))
		} else {
			result.WriteString(" ")
		}
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}
	
	return result.String()
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

// ExecResultMsg represents command execution result
type ExecResultMsg struct {
	Output string
	Err    error
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

// ExecuteCommand executes a shell command
func ExecuteCommand(cmd string) tea.Cmd {
	return func() tea.Msg {
		client := getAIClient()
		if client == nil {
			return ExecResultMsg{Err: fmt.Errorf("AI client not available")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		output, err := client.ExecuteCommand(ctx, cmd)
		return ExecResultMsg{Output: output, Err: err}
	}
}

// ExecuteCommands executes multiple shell commands sequentially
func ExecuteCommands(commands []string) tea.Cmd {
	return func() tea.Msg {
		client := getAIClient()
		if client == nil {
			return ExecResultMsg{Err: fmt.Errorf("AI client not available")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		var allOutput strings.Builder
		var allErrors strings.Builder

		for i, cmd := range commands {
			output, err := client.ExecuteCommand(ctx, cmd)
			if err != nil {
				allErrors.WriteString(fmt.Sprintf("Command %d failed: %v\n", i+1, err))
			} else {
				if output != "" {
					allOutput.WriteString(output)
				}
			}
		}

		result := allOutput.String()
		if errStr := allErrors.String(); errStr != "" {
			return ExecResultMsg{Output: result, Err: fmt.Errorf("%s", errStr)}
		}
		return ExecResultMsg{Output: result, Err: nil}
	}
}

// extractCommands extracts all COMMAND: lines from text
func extractCommands(text string) []string {
	var commands []string
	lines := strings.Split(text, "\n")
	
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "COMMAND:") {
			cmd := strings.TrimSpace(strings.TrimPrefix(line, "COMMAND:"))
			if cmd != "" {
				commands = append(commands, cmd)
			}
		}
	}
	
	return commands
}

// filterValidCommands filters out commands that look like natural language
func filterValidCommands(commands []string) []string {
	var valid []string
	
	for _, cmd := range commands {
		// Skip if it looks like natural language (no shell command structure)
		if isNaturalLanguage(cmd) {
			continue
		}
		valid = append(valid, cmd)
	}
	
	return valid
}

// isNaturalLanguage checks if text looks like natural language instead of a shell command
func isNaturalLanguage(text string) bool {
	// Common shell command starters
	cmdStarters := []string{
		"ls", "cd", "cat", "echo", "grep", "find", "sed", "awk", "cp", "mv", "rm",
		"mkdir", "touch", "chmod", "chown", "git", "npm", "go", "python", "node",
		"curl", "wget", "tar", "zip", "unzip", "head", "tail", "wc", "sort", "uniq",
		"ps", "kill", "top", "htop", "df", "du", "free", "who", "pwd", "env",
		"export", "source", "bash", "sh", "zsh", "fish", "apt", "yum", "brew",
		"docker", "kubectl", "terraform", "ansible", "make", "gcc", "g++",
	}
	
	textLower := strings.ToLower(text)
	
	// Check if it starts with a known command
	for _, starter := range cmdStarters {
		if strings.HasPrefix(textLower, starter+" ") || strings.HasPrefix(textLower, starter) {
			return false
		}
	}
	
	// Check for shell operators (indicates it's a command)
	shellOps := []string{">", "<", "|", "&&", "||", ";", "$", "`", ">>"}
	for _, op := range shellOps {
		if strings.Contains(text, op) {
			return false
		}
	}
	
	// Check for common natural language patterns
	nlPatterns := []string{
		"please", "verify", "check if", "ensure", "make sure", "confirm",
		"you should", "you can", "try to", "need to", "want to",
		"ask user", "prompt user", "request", "describe", "explain",
	}
	
	for _, pattern := range nlPatterns {
		if strings.Contains(textLower, pattern) {
			return true
		}
	}
	
	// If it's a long sentence without shell syntax, likely natural language
	words := strings.Fields(text)
	if len(words) > 8 && !strings.Contains(text, " ") {
		return true
	}
	
	return false
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
