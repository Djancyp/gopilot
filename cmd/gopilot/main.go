package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gopilot/gopilot/client"
	"github.com/gopilot/gopilot/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// Global client for use in commands
var aiClient *client.Client

func main() {
	// Set the client getter function
	ui.SetClientFunc = GetClient

	// Initialize AI client in background to not block UI
	initCmd := func() tea.Msg {
		cfg := client.DefaultConfig()
		c, err := client.New(cfg)
		if err != nil {
			return ui.InitClientMsg{Err: err}
		}
		aiClient = c
		return ui.InitClientMsg{}
	}

	// Create UI model without client initially
	model := ui.NewModel(nil)

	p := tea.NewProgram(&model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Start initialization in background
	go func() {
		msg := initCmd()
		p.Send(msg)
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}

	// Cleanup
	if aiClient != nil {
		aiClient.Unload(context.Background())
	}
}

// GetClient returns the global AI client
func GetClient() *client.Client {
	return aiClient
}
