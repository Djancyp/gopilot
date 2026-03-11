package cmdexec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// CommandExecutor generates shell commands based on user requests
type CommandExecutor struct {
	krn       *kronk.Kronk
	systemCtx SystemContext
}

// SystemContext holds system information for command generation
type SystemContext struct {
	PWD        string
	OS         string
	Arch       string
	Shell      string
	DirListing string
}

// NewCommandExecutor creates a new command executor
func NewCommandExecutor(krn *kronk.Kronk) (*CommandExecutor, error) {
	ctx, err := collectSystemContext()
	if err != nil {
		return nil, fmt.Errorf("collecting system context: %w", err)
	}

	return &CommandExecutor{
		krn:       krn,
		systemCtx: ctx,
	}, nil
}

// collectSystemContext gathers system information
func collectSystemContext() (SystemContext, error) {
	ctx := SystemContext{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// Get current working directory
	pwd, err := os.Getwd()
	if err != nil {
		return ctx, fmt.Errorf("getting PWD: %w", err)
	}
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
	listing, err := getDirListing(pwd)
	if err != nil {
		listing = "Unable to list directory"
	}
	ctx.DirListing = listing

	return ctx, nil
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

// RefreshContext updates the system context
func (e *CommandExecutor) RefreshContext() error {
	ctx, err := collectSystemContext()
	if err != nil {
		return err
	}
	e.systemCtx = ctx
	return nil
}

// GenerateCommand generates a shell command based on user request
func (e *CommandExecutor) GenerateCommand(ctx context.Context, userRequest string) (string, error) {
	systemPrompt := fmt.Sprintf(`You are a shell command generator for a %s system (%s architecture).
Current working directory: %s
Shell: %s

Directory contents:
%s

Generate ONLY the exact shell command to fulfill the user's request.
Do not include explanations, markdown, quotes, or any other text.
Just output the command itself.

Examples:
- "list files" -> ls -la
- "find go files" -> find . -name "*.go"
- "show git status" -> git status
- "create directory test" -> mkdir test
- "read file README.md" -> cat README.md
- "search for function main" -> grep -r "func main" --include="*.go"
- "show disk usage" -> df -h
- "compress logs" -> tar -czvf logs.tar.gz *.log
`, e.systemCtx.OS, e.systemCtx.Arch, e.systemCtx.PWD, e.systemCtx.Shell, e.systemCtx.DirListing)

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage("system", systemPrompt),
			model.TextMessage("user", userRequest),
		),
		"temperature": 0.1,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  256,
	}

	ch, err := e.krn.ChatStreaming(ctx, d)
	if err != nil {
		return "", fmt.Errorf("chat streaming: %w", err)
	}

	var command strings.Builder
	var reasoning bool

	for resp := range ch {
		if len(resp.Choices) == 0 {
			continue
		}

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return "", fmt.Errorf("model error: %s", resp.Choices[0].Delta.Content)
		case model.FinishReasonStop:
			// Clean up the command
			cmd := strings.TrimSpace(command.String())
			cmd = strings.Trim(cmd, "`")
			cmd = strings.TrimSpace(cmd)
			// Remove common prefixes
			cmd = strings.TrimPrefix(cmd, "Command:")
			cmd = strings.TrimPrefix(cmd, "command:")
			cmd = strings.TrimSpace(cmd)
			return cmd, nil
		default:
			delta := resp.Choices[0].Delta
			if delta.Reasoning != "" {
				reasoning = true
				continue
			}
			if reasoning && delta.Content != "" {
				reasoning = false
			}
			if delta.Content != "" && !reasoning {
				command.WriteString(delta.Content)
			}
		}
	}

	cmd := strings.TrimSpace(command.String())
	cmd = strings.Trim(cmd, "`")
	cmd = strings.TrimSpace(cmd)
	cmd = strings.TrimPrefix(cmd, "Command:")
	cmd = strings.TrimPrefix(cmd, "command:")
	return strings.TrimSpace(cmd), nil
}

// ExecuteCommand generates and executes a command
func (e *CommandExecutor) ExecuteCommand(ctx context.Context, userRequest string) (string, error) {
	command, err := e.GenerateCommand(ctx, userRequest)
	if err != nil {
		return "", err
	}

	if command == "" {
		return "", fmt.Errorf("no command generated")
	}

	// Execute the command
	cmd := exec.CommandContext(ctx, e.systemCtx.Shell, "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// GenerateAndRun creates a command executor and runs a command
func GenerateAndRun(krn *kronk.Kronk, userRequest string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	executor, err := NewCommandExecutor(krn)
	if err != nil {
		return "", err
	}

	return executor.ExecuteCommand(ctx, userRequest)
}
