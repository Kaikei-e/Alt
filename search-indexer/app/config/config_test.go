package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "valid configuration",
			envVars: map[string]string{
				"DB_HOST":                    "localhost",
				"DB_PORT":                    "5432",
				"DB_NAME":                    "testdb",
				"SEARCH_INDEXER_DB_USER":     "user",
				"SEARCH_INDEXER_DB_PASSWORD": "pass",
				"MEILISEARCH_HOST":           "http://localhost:7700",
				"MEILISEARCH_API_KEY":        "key",
			},
			wantErr: false,
		},
		{
			name: "missing required env var",
			envVars: map[string]string{
				"DB_HOST": "localhost",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearEnv()

			// Set test environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}
			defer clearEnv()

			if tt.wantErr {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Load() should have panicked but didn't")
					}
				}()
			}

			cfg, err := Load()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Validate configuration values
			if cfg.Database.Host != "localhost" {
				t.Errorf("Database.Host = %v, want localhost", cfg.Database.Host)
			}
			if cfg.Database.Timeout != 10*time.Second {
				t.Errorf("Database.Timeout = %v, want 10s", cfg.Database.Timeout)
			}
			if cfg.HTTP.Addr != ":9300" {
				t.Errorf("HTTP.Addr = %v, want :9300", cfg.HTTP.Addr)
			}
		})
	}
}

func TestDatabaseConfig_ConnectionString(t *testing.T) {
	cfg := &DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "user",
		Password: "pass",
		Name:     "testdb",
	}

	want := "host=localhost port=5432 user=user password=pass dbname=testdb sslmode=disable"
	got := cfg.ConnectionString()

	if got != want {
		t.Errorf("ConnectionString() = %v, want %v", got, want)
	}
}

func clearEnv() {
	vars := []string{
		"DB_HOST", "DB_PORT", "DB_NAME", "SEARCH_INDEXER_DB_USER", "SEARCH_INDEXER_DB_PASSWORD",
		"MEILISEARCH_HOST", "MEILISEARCH_API_KEY",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}
