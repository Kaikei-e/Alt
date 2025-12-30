package config

import (
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
			// Set test environment variables using t.Setenv (auto-cleanup)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
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
		SSL:      SSLConfig{Mode: "prefer"}, // SSL設定を追加
	}

	want := "host=localhost port=5432 user=user password=pass dbname=testdb sslmode=prefer"
	got := cfg.ConnectionString()

	if got != want {
		t.Errorf("ConnectionString() = %v, want %v", got, want)
	}
}
