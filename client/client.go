package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/gopilot/gopilot/logger"
)

// SystemContext holds system information for command generation
type SystemContext struct {
	PWD        string
	OS         string
	Arch       string
	Shell      string
	DirListing string
}

// Client wraps the Kronk AI client
type Client struct {
	krn       *kronk.Kronk
	messages  []model.D
	modelName string
	sysCtx    SystemContext
}

// suppressStderr temporarily redirects stderr to /dev/null
func suppressStderr() func() {
	// Save original stderr
	origStderr, _ := syscall.Dup(int(os.Stderr.Fd()))
	
	// Open /dev/null
	null, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
	if err != nil {
		return func() {}
	}
	
	// Redirect stderr to /dev/null
	syscall.Dup2(int(null.Fd()), int(os.Stderr.Fd()))
	
	return func() {
		// Restore original stderr
		if origStderr >= 0 {
			syscall.Dup2(origStderr, int(os.Stderr.Fd()))
			syscall.Close(origStderr)
		}
		null.Close()
	}
}

// Config holds the client configuration
type Config struct {
	ModelSourceURL string
	ModelID        string
	SystemPrompt   string
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		ModelSourceURL: "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf",
		SystemPrompt: `You are GoPilot, a helpful AI assistant integrated into a terminal chat application.
You help users with coding questions, explain concepts, and provide clear, concise answers.

For technical tasks (file operations, system commands, git, searches, etc.), provide shell commands to execute.

IMPORTANT FORMAT - Each command MUST be on its own line starting with COMMAND:
PLAN: <brief explanation>
COMMAND: <exact shell command - one per line>
COMMAND: <another command if needed>
SUMMARY: <what was accomplished>

GOOD Examples:
User: "add tailwind to index.html"
PLAN: Add Tailwind CSS CDN to index.html
COMMAND: sed -i '/<\/head>/i\  <script src="https://cdn.tailwindcss.com"></script>' index.html
SUMMARY: Added Tailwind CSS CDN script before </head> tag

User: "create css file"
PLAN: Create index.css with default styles
COMMAND: cat > index.css << 'EOF'
body { margin: 0; font-family: system-ui, sans-serif; }
.container { max-width: 1200px; margin: 0 auto; }
EOF
SUMMARY: Created index.css with base styles

User: "list go files"
PLAN: Find all Go files
COMMAND: find . -name "*.go" -type f
SUMMARY: Listed all Go source files

BAD Examples (DO NOT DO THIS):
- Don't use numbered lists like "1. Verify..."
- Don't use markdown code blocks for commands
- Don't write explanations instead of commands
- Each COMMAND: must be a single executable shell command

Always use COMMAND: prefix for each shell command you want executed.`,
	}
}

// New creates a new AI client
func New(cfg Config) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	// Suppress verbose logging by redirecting stdout temporarily
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Initialize libraries
	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		w.Close()
		os.Stdout = oldStdout
		return nil, fmt.Errorf("failed to init libs: %w", err)
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		w.Close()
		os.Stdout = oldStdout
		return nil, fmt.Errorf("failed to download libs: %w", err)
	}

	// Initialize catalog (optional but recommended)
	ctlg, err := catalog.New()
	if err != nil {
		w.Close()
		os.Stdout = oldStdout
		return nil, fmt.Errorf("failed to create catalog: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		w.Close()
		os.Stdout = oldStdout
		return nil, fmt.Errorf("failed to download catalog: %w", err)
	}

	// Download model
	mdls, err := models.New()
	if err != nil {
		w.Close()
		os.Stdout = oldStdout
		return nil, fmt.Errorf("failed to init models: %w", err)
	}

	var mp models.Path
	switch {
	case cfg.ModelSourceURL != "":
		mp, err = mdls.Download(ctx, kronk.FmtLogger, cfg.ModelSourceURL, "")
	case cfg.ModelID != "":
		mp, err = ctlg.DownloadModel(ctx, kronk.FmtLogger, cfg.ModelID)
	default:
		w.Close()
		os.Stdout = oldStdout
		return nil, fmt.Errorf("no model source specified")
	}

	if err != nil {
		w.Close()
		os.Stdout = oldStdout
		return nil, fmt.Errorf("failed to download model: %w", err)
	}

	// Restore stdout and discard the captured output
	w.Close()
	os.Stdout = oldStdout
	io.Copy(io.Discard, r)

	// Suppress stderr for kronk init and model loading (C library logs)
	restoreStderr := suppressStderr()
	defer restoreStderr()

	// Initialize Kronk
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("failed to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles: mp.ModelFiles,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kronk: %w", err)
	}

	// Restore stderr after kronk is initialized
	restoreStderr()

	// Extract model name from path
	modelName := ""
	if len(mp.ModelFiles) > 0 {
		modelName = mp.ModelFiles[0]
		if idx := strings.LastIndex(modelName, "/"); idx >= 0 {
			modelName = modelName[idx+1:]
		}
	}

	// Collect system context
	sysCtx := collectSystemContext()

	// Initialize messages with system prompt
	messages := []model.D{
		model.TextMessage(model.RoleSystem, cfg.SystemPrompt+"\n\nSystem Context:\n- OS: "+sysCtx.OS+"\n- Arch: "+sysCtx.Arch+"\n- PWD: "+sysCtx.PWD+"\n- Shell: "+sysCtx.Shell),
	}

	return &Client{
		krn:       krn,
		messages:  messages,
		modelName: modelName,
		sysCtx:    sysCtx,
	}, nil
}

// collectSystemContext gathers system information
func collectSystemContext() SystemContext {
	ctx := SystemContext{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// Get current working directory
	pwd, _ := os.Getwd()
	ctx.PWD = pwd

	// Determine shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "cmd.exe"
		} else {
			shell = "/bin/sh"
		}
	}
	ctx.Shell = shell

	// Get directory listing
	listing, _ := getDirListing(pwd)
	ctx.DirListing = listing

	return ctx
}

// getDirListing gets a brief directory listing
func getDirListing(path string) (string, error) {
	cmd := exec.Command("ls", "-la", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// ChatResponse represents a streaming response
type ChatResponse struct {
	Content    string
	Reasoning  string
	IsComplete bool
	Err        error
}

// Chat sends a message and returns a streaming response channel
func (c *Client) Chat(ctx context.Context, userMessage string) (<-chan ChatResponse, error) {
	// Add user message to history
	c.messages = append(c.messages, model.TextMessage(model.RoleUser, userMessage))

	// Prepare request
	d := model.D{
		"messages":   c.messages,
		"max_tokens": 2048,
	}

	// Start streaming chat
	ch, err := c.krn.ChatStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	// Transform kronk response channel to our response channel
	respCh := make(chan ChatResponse, 100)

	go func() {
		defer close(respCh)

		var accumulatedContent strings.Builder
		var accumulatedReasoning strings.Builder

		for resp := range ch {
			if len(resp.Choices) == 0 {
				continue
			}

			delta := resp.Choices[0].Delta

			switch resp.Choices[0].FinishReason() {
			case model.FinishReasonError:
				respCh <- ChatResponse{Err: fmt.Errorf("model error: %s", delta.Content)}
				return

			case model.FinishReasonStop:
				// Store assistant response in history
				finalContent := accumulatedContent.String()
				if finalContent != "" {
					c.messages = append(c.messages, model.TextMessage(model.RoleAssistant, finalContent))
				}
				respCh <- ChatResponse{IsComplete: true}
				return

			default:
				if delta.Reasoning != "" {
					accumulatedReasoning.WriteString(delta.Reasoning)
					respCh <- ChatResponse{Reasoning: delta.Reasoning}
				}
				if delta.Content != "" {
					accumulatedContent.WriteString(delta.Content)
					respCh <- ChatResponse{Content: delta.Content}
				}
			}
		}

		// Channel closed without explicit stop - send accumulated content
		finalContent := accumulatedContent.String()
		if finalContent != "" {
			c.messages = append(c.messages, model.TextMessage(model.RoleAssistant, finalContent))
		}
		respCh <- ChatResponse{IsComplete: true}
	}()

	return respCh, nil
}

// Unload unloads the model from memory
func (c *Client) Unload(ctx context.Context) error {
	if c.krn != nil {
		return c.krn.Unload(ctx)
	}
	return nil
}

// GetStats returns current session stats
func (c *Client) GetStats() map[string]string {
	return c.krn.SystemInfo()
}

// GetModelName returns the model name
func (c *Client) GetModelName() string {
	return c.modelName
}

// Reset clears the conversation history
func (c *Client) Reset(systemPrompt string) {
	c.messages = []model.D{
		model.TextMessage(model.RoleSystem, systemPrompt),
	}
}

// GetHistoryLength returns the number of messages in history
func (c *Client) GetHistoryLength() int {
	return len(c.messages)
}

// ExecuteCommand executes a shell command and returns the output
func (c *Client) ExecuteCommand(ctx context.Context, cmd string) (string, error) {
	logger.Info("Executing command", "command", cmd, "shell", c.sysCtx.Shell, "pwd", c.sysCtx.PWD)

	shell := c.sysCtx.Shell
	if shell == "" {
		shell = "/bin/sh"
	}
	
	execCmd := exec.CommandContext(ctx, shell, "-c", cmd)
	execCmd.Dir = c.sysCtx.PWD  // Set working directory
	output, err := execCmd.CombinedOutput()

	if err != nil {
		logger.Error("Command failed", "command", cmd, "error", err, "output", string(output))
		return "", fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}

	logger.Info("Command succeeded", "command", cmd, "output_length", len(output), "output", string(output))
	return string(output), nil
}

// GetSystemContext returns the current system context
func (c *Client) GetSystemContext() SystemContext {
	return c.sysCtx
}

// RefreshSystemContext updates the system context
func (c *Client) RefreshSystemContext() {
	c.sysCtx = collectSystemContext()
}
