package config

import (
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "valid configuration with backend API",
			envVars: map[string]string{
				"BACKEND_API_URL": "http://alt-backend:9101",
				"MEILISEARCH_HOST": "http://localhost:7700",
				"MEILISEARCH_API_KEY": "key",
			},
			wantErr: false,
		},
		{
			name: "missing BACKEND_API_URL",
			envVars: map[string]string{
				"MEILISEARCH_HOST": "http://localhost:7700",
			},
			wantErr: true,
		},
		{
			name: "missing MEILISEARCH_HOST",
			envVars: map[string]string{
				"BACKEND_API_URL": "http://alt-backend:9101",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			if cfg.BackendAPI.URL != "http://alt-backend:9101" {
				t.Errorf("BackendAPI.URL = %v, want http://alt-backend:9101", cfg.BackendAPI.URL)
			}
			if cfg.HTTP.Addr != ":9300" {
				t.Errorf("HTTP.Addr = %v, want :9300", cfg.HTTP.Addr)
			}
		})
	}
}
