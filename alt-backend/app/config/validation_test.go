package config

import (
	"testing"
)

func TestValidateAuthConfig_ProductionRequiresSecrets(t *testing.T) {
	tests := []struct {
		name    string
		config  AuthConfig
		env     string
		wantErr bool
		errMsg  string
	}{
		{
			name: "production without shared secret fails",
			config: AuthConfig{
				SharedSecret:       "",
				BackendTokenSecret: "this-is-a-very-long-secret-for-testing-purposes",
			},
			env:     "production",
			wantErr: true,
			errMsg:  "AUTH_SHARED_SECRET is required in production",
		},
		{
			name: "production without backend token secret fails",
			config: AuthConfig{
				SharedSecret:       "this-is-a-very-long-secret-for-testing-purposes",
				BackendTokenSecret: "",
			},
			env:     "production",
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_SECRET is required in production",
		},
		{
			name: "production with short shared secret fails",
			config: AuthConfig{
				SharedSecret:       "short",
				BackendTokenSecret: "this-is-a-very-long-secret-for-testing-purposes",
			},
			env:     "production",
			wantErr: true,
			errMsg:  "AUTH_SHARED_SECRET must be at least 32 characters",
		},
		{
			name: "production with short backend token secret fails",
			config: AuthConfig{
				SharedSecret:       "this-is-a-very-long-secret-for-testing-purposes",
				BackendTokenSecret: "short",
			},
			env:     "production",
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_SECRET must be at least 32 characters",
		},
		{
			name: "production with valid secrets passes",
			config: AuthConfig{
				SharedSecret:       "this-is-a-very-long-secret-for-testing-purposes",
				BackendTokenSecret: "this-is-another-very-long-secret-for-testing-12",
			},
			env:     "production",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAuthConfig(&tt.config, tt.env)
			if tt.wantErr {
				if err == nil {
					t.Error("validateAuthConfig() expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateAuthConfig() error = %v, want to contain %s", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateAuthConfig() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateAuthConfig_DevelopmentAllowsEmptySecrets(t *testing.T) {
	tests := []struct {
		name    string
		config  AuthConfig
		env     string
		wantErr bool
	}{
		{
			name: "development without secrets is OK",
			config: AuthConfig{
				SharedSecret:       "",
				BackendTokenSecret: "",
			},
			env:     "development",
			wantErr: false,
		},
		{
			name: "empty env (defaults to development) without secrets is OK",
			config: AuthConfig{
				SharedSecret:       "",
				BackendTokenSecret: "",
			},
			env:     "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAuthConfig(&tt.config, tt.env)
			if tt.wantErr {
				if err == nil {
					t.Error("validateAuthConfig() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("validateAuthConfig() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateAuthConfig_ShortSecretInDevelopment(t *testing.T) {
	tests := []struct {
		name    string
		config  AuthConfig
		env     string
		wantErr bool
		errMsg  string
	}{
		{
			name: "development with too short shared secret fails",
			config: AuthConfig{
				SharedSecret:       "tooshort",
				BackendTokenSecret: "",
			},
			env:     "development",
			wantErr: true,
			errMsg:  "AUTH_SHARED_SECRET is too short",
		},
		{
			name: "development with too short backend token secret fails",
			config: AuthConfig{
				SharedSecret:       "",
				BackendTokenSecret: "tooshort",
			},
			env:     "development",
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_SECRET is too short",
		},
		{
			name: "development with 16+ char secrets passes",
			config: AuthConfig{
				SharedSecret:       "exactly-16-chars!",
				BackendTokenSecret: "exactly-16-chars!",
			},
			env:     "development",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAuthConfig(&tt.config, tt.env)
			if tt.wantErr {
				if err == nil {
					t.Error("validateAuthConfig() expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateAuthConfig() error = %v, want to contain %s", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateAuthConfig() unexpected error: %v", err)
				}
			}
		})
	}
}
