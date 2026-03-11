package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ListDir executes ls -la for a given directory
func ListDir(path string) (string, error) {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	// Clean path
	path = filepath.Clean(path)

	cmd := exec.Command("ls", "-la", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// ReadFile reads the contents of a file
func ReadFile(path string) (string, error) {
	if path == "" {
		return "", os.ErrInvalid
	}

	// Clean path
	path = filepath.Clean(path)

	// If path is relative, make it absolute from current directory
	if !filepath.IsAbs(path) {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}
	}

	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	// Don't read files larger than 1MB
	if info.Size() > 1024*1024 {
		return "", fmt.Errorf("file too large (max 1MB)")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("unable to read file: %w", err)
	}

	return string(content), nil
}

// WriteFile writes content to a file
func WriteFile(path, content string) (string, error) {
	if path == "" {
		return "", os.ErrInvalid
	}

	// Clean path
	path = filepath.Clean(path)

	// Create parent directories if they don't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// Write file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}

	return "File written successfully: " + path, nil
}

// SearchFiles searches for files matching a pattern
func SearchFiles(pattern, dir string) (string, error) {
	if pattern == "" {
		return "", os.ErrInvalid
	}

	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	// Clean path
	dir = filepath.Clean(dir)

	var matches []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if matched, _ := filepath.Match(pattern, info.Name()); matched {
			matches = append(matches, path)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	if len(matches) == 0 {
		return "No files found matching: " + pattern, nil
	}

	result := "Found " + string(rune(len(matches))) + " file(s):\n"
	for _, m := range matches {
		result += m + "\n"
	}

	return result, nil
}
