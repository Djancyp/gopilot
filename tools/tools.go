package tools

import (
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
