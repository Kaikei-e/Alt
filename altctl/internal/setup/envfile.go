package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateEnvFile copies .env.example to .env in the given project root.
// Returns true if the file was created. Existing .env is skipped unless force is true.
func CreateEnvFile(projectRoot string, force bool) (bool, error) {
	examplePath := filepath.Join(projectRoot, ".env.example")
	envPath := filepath.Join(projectRoot, ".env")

	// Check .env.example exists
	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		return false, fmt.Errorf(".env.example not found in %s", projectRoot)
	}

	// Skip if .env exists and not forcing
	if !force {
		if _, err := os.Stat(envPath); err == nil {
			return false, nil
		}
	}

	content, err := os.ReadFile(examplePath)
	if err != nil {
		return false, fmt.Errorf("reading .env.example: %w", err)
	}

	if err := os.WriteFile(envPath, content, 0644); err != nil {
		return false, fmt.Errorf("writing .env: %w", err)
	}

	return true, nil
}
