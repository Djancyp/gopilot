package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors - Simple monochrome
	TextColor      = lipgloss.Color("#FFFFFF")
	MutedColor     = lipgloss.Color("#808080")
	AccentColor   = lipgloss.Color("#00FF00")
	ErrorColor    = lipgloss.Color("#FF0000")
	ScrollbarColor = lipgloss.Color("#FFA500")

	// Base styles
	BaseStyle = lipgloss.NewStyle().
			Foreground(TextColor)

	// Title styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(AccentColor).
			Bold(true).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(MutedColor)

	// Message styles
	UserMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Padding(0, 2).
				MarginBottom(1)

	AssistantMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#333333")).
				Padding(0, 2).
				MarginBottom(1)

	ReasoningMessageStyle = lipgloss.NewStyle().
				Foreground(MutedColor).
				Italic(true).
				MarginBottom(1)

	SystemMessageStyle = lipgloss.NewStyle().
				Foreground(MutedColor).
				Italic(true).
				MarginBottom(1)

	ErrorMessageStyle = lipgloss.NewStyle().
				Foreground(ErrorColor).
				MarginBottom(1)

	// Input styles
	InputStyle = lipgloss.NewStyle().
			Foreground(TextColor)

	InputPromptStyle = lipgloss.NewStyle().
				Foreground(AccentColor)

	// Container styles
	ContainerStyle = lipgloss.NewStyle().
			Padding(0, 0)

	ChatContainerStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#333333")).
				Padding(1, 2)

	HelpStyle = lipgloss.NewStyle().
			Foreground(MutedColor).
			MarginTop(1)

	// Status styles
	LoadingStyle = lipgloss.NewStyle().
			Foreground(MutedColor)

	ReadyStyle = lipgloss.NewStyle().
			Foreground(AccentColor)
)
