package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironment_String(t *testing.T) {
	tests := []struct {
		name string
		env  Environment
		want string
	}{
		{
			name: "development environment",
			env:  Development,
			want: "development",
		},
		{
			name: "staging environment",
			env:  Staging,
			want: "staging",
		},
		{
			name: "production environment",
			env:  Production,
			want: "production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.env.String())
		})
	}
}

func TestEnvironment_IsValid(t *testing.T) {
	tests := []struct {
		name string
		env  Environment
		want bool
	}{
		{
			name: "development is valid",
			env:  Development,
			want: true,
		},
		{
			name: "staging is valid",
			env:  Staging,
			want: true,
		},
		{
			name: "production is valid",
			env:  Production,
			want: true,
		},
		{
			name: "invalid environment",
			env:  Environment("invalid"),
			want: false,
		},
		{
			name: "empty environment",
			env:  Environment(""),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.env.IsValid())
		})
	}
}

func TestParseEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Environment
		wantErr bool
	}{
		{
			name:    "parse development",
			input:   "development",
			want:    Development,
			wantErr: false,
		},
		{
			name:    "parse staging",
			input:   "staging",
			want:    Staging,
			wantErr: false,
		},
		{
			name:    "parse production",
			input:   "production",
			want:    Production,
			wantErr: false,
		},
		{
			name:    "parse uppercase development",
			input:   "DEVELOPMENT",
			want:    Development,
			wantErr: false,
		},
		{
			name:    "parse mixed case staging",
			input:   "StAgInG",
			want:    Staging,
			wantErr: false,
		},
		{
			name:    "parse invalid environment",
			input:   "invalid",
			want:    Environment(""),
			wantErr: true,
		},
		{
			name:    "parse empty string",
			input:   "",
			want:    Environment(""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnvironment(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestEnvironment_DefaultNamespace(t *testing.T) {
	tests := []struct {
		name string
		env  Environment
		want string
	}{
		{
			name: "development default namespace",
			env:  Development,
			want: "alt-dev",
		},
		{
			name: "staging default namespace",
			env:  Staging,
			want: "alt-staging",
		},
		{
			name: "production default namespace",
			env:  Production,
			want: "alt-production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.env.DefaultNamespace())
		})
	}
}
