package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/gopilot/gopilot/client"
	"github.com/gopilot/gopilot/internal/ui"
	"github.com/gopilot/gopilot/logger"

	tea "github.com/charmbracelet/bubbletea"
)

// Global client for use in commands
var aiClient *client.Client

func main() {
	// Parse flags
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Initialize logger
	if err := logger.Init(*debug); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to init logger: %v\n", err)
	}
	defer logger.Close()

	logger.Info("GoPilot starting", "debug", *debug)

	// Set the client getter function
	ui.SetClientFunc = GetClient

	// Initialize AI client in background to not block UI
	initCmd := func() tea.Msg {
		cfg := client.DefaultConfig()
		c, err := client.New(cfg)
		if err != nil {
			logger.Error("Failed to create client", "error", err)
			return ui.InitClientMsg{Err: err}
		}
		aiClient = c
		logger.Info("Client initialized", "model", c.GetModelName())
		return ui.InitClientMsg{}
	}

	// Create UI model without client initially
	model := ui.NewModel(nil)

	p := tea.NewProgram(&model, tea.WithAltScreen())

	// Start initialization in background
	go func() {
		msg := initCmd()
		p.Send(msg)
	}()

	if _, err := p.Run(); err != nil {
		logger.Error("Program error", "error", err)
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}

	// Cleanup
	if aiClient != nil {
		if err := aiClient.Unload(context.Background()); err != nil {
			logger.Error("Failed to unload client", "error", err)
		}
	}

	logger.Info("GoPilot exited")
}

// GetClient returns the global AI client
func GetClient() *client.Client {
	return aiClient
}
