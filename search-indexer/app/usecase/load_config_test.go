package usecase

import (
	"context"
	"search-indexer/domain"
	"search-indexer/port"
	"testing"
	"time"
)

// Mock config repository for testing
type mockConfigRepository struct {
	config *domain.SearchIndexerConfig
	err    error
}

func (m *mockConfigRepository) LoadSearchIndexerConfig() (*domain.SearchIndexerConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.config, nil
}

func TestLoadConfigUsecase_Execute(t *testing.T) {
	validConfig, _ := domain.NewSearchIndexerConfig(
		"postgresql://user:pass@localhost:5432/testdb",
		"http://localhost:7700",
		"test-key",
	)

	tests := []struct {
		name        string
		mockConfig  *domain.SearchIndexerConfig
		mockError   error
		wantErr     bool
		validateCfg func(*domain.SearchIndexerConfig) bool
	}{
		{
			name:       "successful config loading",
			mockConfig: validConfig,
			mockError:  nil,
			wantErr:    false,
			validateCfg: func(cfg *domain.SearchIndexerConfig) bool {
				return cfg.DatabaseURL() == "postgresql://user:pass@localhost:5432/testdb" &&
					cfg.MeilisearchHost() == "http://localhost:7700" &&
					cfg.MeilisearchAPIKey() == "test-key" &&
					cfg.IndexInterval() == 1*time.Minute &&
					cfg.BatchSize() == 200
			},
		},
		{
			name:       "repository error",
			mockConfig: nil,
			mockError:  &port.RepositoryError{Op: "LoadSearchIndexerConfig", Err: "env var not set"},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockConfigRepository{
				config: tt.mockConfig,
				err:    tt.mockError,
			}

			usecase := NewLoadConfigUsecase(repo)

			result, err := usecase.Execute(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateCfg != nil && !tt.validateCfg(result.Config) {
				t.Errorf("Execute() config validation failed")
			}
		})
	}
}
