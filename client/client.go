package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/gopilot/gopilot/tools"
)

// Tool describes the features which all tools must implement.
type Tool interface {
	Call(ctx context.Context, toolCall model.ResponseToolCall) model.D
}

// listDirectoryTool implements the list_directory tool
type listDirectoryTool struct{}

func (t listDirectoryTool) Call(ctx context.Context, toolCall model.ResponseToolCall) model.D {
	var args struct {
		Path string `json:"path"`
	}
	argsJSON, _ := json.Marshal(toolCall.Function.Arguments)
	json.Unmarshal(argsJSON, &args)

	result, err := tools.ListDir(args.Path)
	status := "SUCCEEDED"
	if err != nil {
		status = "FAILED"
		result = err.Error()
	}

	return model.D{
		"tool_call_id": toolCall.ID,
		"role":         "tool",
		"status":       status,
		"data":         result,
	}
}

func RegisterListDirectory(toolsMap map[string]Tool) model.D {
	toolsMap["list_directory"] = listDirectoryTool{}
	return model.D{
		"type": "function",
		"function": model.D{
			"name":        "list_directory",
			"description": "List directory contents with details (like ls -la)",
			"parameters": model.D{
				"type": "object",
				"properties": model.D{
					"path": model.D{
						"type":        "string",
						"description": "Directory path to list, defaults to current directory if not provided",
					},
				},
			},
		},
	}
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

// Client wraps the Kronk AI client
type Client struct {
	krn       *kronk.Kronk
	messages  []model.D
	modelName string
	tools     map[string]Tool
	toolDocs  []model.D
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
Always be helpful and accurate in your responses.

You have access to the following tools:
- list_directory: List directory contents with details (like ls -la)
  Parameters: path (string, optional) - directory path, defaults to current directory`,
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

	// Initialize messages with system prompt
	messages := []model.D{
		model.TextMessage(model.RoleSystem, cfg.SystemPrompt),
	}

	// Initialize tools
	toolsMap := make(map[string]Tool)
	toolDocs := []model.D{
		RegisterListDirectory(toolsMap),
	}

	return &Client{
		krn:       krn,
		messages:  messages,
		modelName: modelName,
		tools:     toolsMap,
		toolDocs:  toolDocs,
	}, nil
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

	// Prepare request with tools
	d := model.D{
		"messages":       c.messages,
		"tools":          c.toolDocs,
		"tool_selection": "auto",
		"max_tokens":     2048,
	}

	// Start streaming chat
	ch, err := c.krn.ChatStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	// Transform kronk response channel to our response channel
	respCh := make(chan ChatResponse)

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

			case model.FinishReasonTool:
				// Handle tool calls
				if delta.ToolCalls != nil && len(delta.ToolCalls) > 0 {
					// Add tool call request to conversation
					var toolCallDocs []model.D
					for _, tc := range delta.ToolCalls {
						argsJSON, _ := json.Marshal(tc.Function.Arguments)
						toolCallDocs = append(toolCallDocs, model.D{
							"id":   tc.ID,
							"type": "function",
							"function": model.D{
								"name":      tc.Function.Name,
								"arguments": string(argsJSON),
							},
						})
					}
					c.messages = append(c.messages, model.D{
						"role":       "assistant",
						"tool_calls": toolCallDocs,
					})

					// Execute tools
					for _, toolCall := range delta.ToolCalls {
						tool, exists := c.tools[toolCall.Function.Name]
						if !exists {
							continue
						}

						// Call the tool
						result := tool.Call(ctx, toolCall)

						// Send tool result to UI
						if status, ok := result["status"].(string); ok && status == "SUCCEEDED" {
							if data, ok := result["data"].(string); ok {
								respCh <- ChatResponse{Content: fmt.Sprintf("```\n%s\n```", data)}
							}
						} else {
							respCh <- ChatResponse{Content: "Tool call failed"}
						}

						// Add tool response to history
						c.messages = append(c.messages, result)
					}
				}

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
