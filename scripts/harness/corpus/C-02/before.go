package config

import "fmt"

type ConfigFileNotFoundError struct {
	Path string
}

func (e ConfigFileNotFoundError) Error() string {
	return "config file not found: " + e.Path
}

func loadConfig(readInConfig func() error) error {
	if err := readInConfig(); err != nil {
		if _, ok := err.(ConfigFileNotFoundError); !ok {
			return fmt.Errorf("reading config: %w", err)
		}
		// Config file not found is OK; fall back to defaults.
	}
	return nil
}
