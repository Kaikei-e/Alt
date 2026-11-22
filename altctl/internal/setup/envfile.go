package setup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alt-project/altctl/internal/fsperm"
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

	// #nosec G304 -- examplePath joins projectRoot with constant ".env.example"
	content, err := os.ReadFile(examplePath)
	if err != nil {
		return false, fmt.Errorf("reading .env.example: %w", err)
	}

	// #nosec G703 -- envPath joins operator-configured projectRoot with constant ".env"
	if err := os.WriteFile(envPath, content, fsperm.SecretFile); err != nil {
		return false, fmt.Errorf("writing .env: %w", err)
	}

	return true, nil
}
