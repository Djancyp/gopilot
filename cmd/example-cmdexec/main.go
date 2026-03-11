// Command executor example - generates and runs shell commands
// Run: go run ./cmd/example-cmdexec/main.go

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/gopilot/gopilot/cmdexec"
)

const modelURL = "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("installing system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("initializing kronk: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading model...")
		krn.Unload(context.Background())
	}()

	fmt.Println("\n=== Command Executor ===")
	fmt.Println("Ask me to run shell commands (type 'quit' to exit)")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\u001b[94mCMD>\u001b[0m ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "quit" || input == "exit" {
			break
		}

		if input == "" {
			continue
		}

		fmt.Println("\u001b[93mGenerating command...\u001b[0m")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		executor, err := cmdexec.NewCommandExecutor(krn)
		if err != nil {
			fmt.Printf("Error creating executor: %v\n", err)
			continue
		}

		command, err := executor.GenerateCommand(ctx, input)
		if err != nil {
			fmt.Printf("Error generating command: %v\n", err)
			continue
		}

		fmt.Printf("\u001b[92mGenerated command:\u001b[0m %s\n", command)

		// Ask for confirmation
		fmt.Print("\u001b[93mExecute? (y/n):\u001b[0m ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(confirm)

		if confirm == "y" || confirm == "yes" {
			fmt.Println("\u001b[93mExecuting...\u001b[0m")
			output, err := executor.ExecuteCommand(ctx, input)
			if err != nil {
				fmt.Printf("\u001b[91mError:\u001b[0m %v\n", err)
			} else {
				fmt.Printf("\u001b[92mOutput:\u001b[0m\n%s\n", output)
			}
		}
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(libs.WithVersion(defaults.LibVersion("")))
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("downloading libs: %w", err)
	}

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, err
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, "")
	if err != nil {
		return models.Path{}, fmt.Errorf("downloading model: %w", err)
	}

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("Loading model...")

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("initializing kronk: %w", err)
	}

	cfg := model.Config{ModelFiles: mp.ModelFiles}
	krn, err := kronk.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kronk: %w", err)
	}

	fmt.Println("Model loaded successfully!")
	return krn, nil
}
