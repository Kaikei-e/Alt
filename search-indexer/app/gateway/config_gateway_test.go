package gateway

import (
	"os"
	"search-indexer/domain"
	"testing"
	"time"
)

// Mock config driver for testing
type mockConfigDriver struct {
	envVars map[string]string
	err     error
}

func (m *mockConfigDriver) GetEnv(key string) string {
	if m.envVars == nil {
		return ""
	}
	return m.envVars[key]
}

func (m *mockConfigDriver) ValidateConnection() error {
	return m.err
}

func TestConfigGateway_LoadSearchIndexerConfig(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		wantErr     bool
		validateCfg func(*domain.SearchIndexerConfig) bool
	}{
		{
			name: "successful config loading with all required env vars",
			envVars: map[string]string{
				"DATABASE_URL":        "postgresql://user:pass@localhost:5432/testdb",
				"MEILISEARCH_HOST":    "http://localhost:7700",
				"MEILISEARCH_API_KEY": "test-key",
			},
			wantErr: false,
			validateCfg: func(cfg *domain.SearchIndexerConfig) bool {
				return cfg.DatabaseURL() == "postgresql://user:pass@localhost:5432/testdb" &&
					cfg.MeilisearchHost() == "http://localhost:7700" &&
					cfg.MeilisearchAPIKey() == "test-key" &&
					cfg.IndexInterval() == 1*time.Minute &&
					cfg.BatchSize() == 200
			},
		},
		{
			name: "missing DATABASE_URL should fail",
			envVars: map[string]string{
				"MEILISEARCH_HOST":    "http://localhost:7700",
				"MEILISEARCH_API_KEY": "test-key",
			},
			wantErr: true,
		},
		{
			name: "missing MEILISEARCH_HOST should fail",
			envVars: map[string]string{
				"DATABASE_URL":        "postgresql://user:pass@localhost:5432/testdb",
				"MEILISEARCH_API_KEY": "test-key",
			},
			wantErr: true,
		},
		{
			name: "empty MEILISEARCH_API_KEY should use default (empty string)",
			envVars: map[string]string{
				"DATABASE_URL":     "postgresql://user:pass@localhost:5432/testdb",
				"MEILISEARCH_HOST": "http://localhost:7700",
			},
			wantErr: false,
			validateCfg: func(cfg *domain.SearchIndexerConfig) bool {
				return cfg.MeilisearchAPIKey() == ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for this test
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer func() {
				// Clean up environment variables
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
				// Also clean up the required ones that might not be in tt.envVars
				os.Unsetenv("DATABASE_URL")
				os.Unsetenv("MEILISEARCH_HOST")
				os.Unsetenv("MEILISEARCH_API_KEY")
			}()

			driver := &mockConfigDriver{
				envVars: tt.envVars,
			}

			gateway := NewConfigGateway(driver)

			config, err := gateway.LoadSearchIndexerConfig()

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadSearchIndexerConfig() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("LoadSearchIndexerConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateCfg != nil && !tt.validateCfg(config) {
				t.Errorf("LoadSearchIndexerConfig() validation failed")
			}
		})
	}
}

func TestConfigGateway_ConvertToDomain(t *testing.T) {
	tests := []struct {
		name            string
		databaseURL     string
		meilisearchHost string
		meilisearchKey  string
		wantErr         bool
		validateCfg     func(*domain.SearchIndexerConfig) bool
	}{
		{
			name:            "valid configuration conversion",
			databaseURL:     "postgresql://user:pass@localhost:5432/testdb",
			meilisearchHost: "http://localhost:7700",
			meilisearchKey:  "test-key",
			wantErr:         false,
			validateCfg: func(cfg *domain.SearchIndexerConfig) bool {
				return cfg.DatabaseURL() == "postgresql://user:pass@localhost:5432/testdb" &&
					cfg.MeilisearchHost() == "http://localhost:7700" &&
					cfg.MeilisearchAPIKey() == "test-key"
			},
		},
		{
			name:            "empty database URL should fail",
			databaseURL:     "",
			meilisearchHost: "http://localhost:7700",
			meilisearchKey:  "test-key",
			wantErr:         true,
		},
		{
			name:            "empty meilisearch host should fail",
			databaseURL:     "postgresql://user:pass@localhost:5432/testdb",
			meilisearchHost: "",
			meilisearchKey:  "test-key",
			wantErr:         true,
		},
	}

	gateway := &ConfigGateway{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := gateway.convertToDomain(tt.databaseURL, tt.meilisearchHost, tt.meilisearchKey)

			if tt.wantErr {
				if err == nil {
					t.Errorf("convertToDomain() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("convertToDomain() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateCfg != nil && !tt.validateCfg(config) {
				t.Errorf("convertToDomain() validation failed")
			}
		})
	}
}
