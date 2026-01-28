package sql

import (
	"database/sql"
	"testing"
	"time"
)

func TestNullStringPtr(t *testing.T) {
	tests := []struct {
		name  string
		input sql.NullString
		want  *string
	}{
		{
			name:  "valid string",
			input: sql.NullString{String: "hello", Valid: true},
			want:  ptr("hello"),
		},
		{
			name:  "invalid string",
			input: sql.NullString{String: "hello", Valid: false},
			want:  nil,
		},
		{
			name:  "empty valid string",
			input: sql.NullString{String: "", Valid: true},
			want:  ptr(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NullStringPtr(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("NullStringPtr() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Errorf("NullStringPtr() = nil, want %v", *tt.want)
				return
			}
			if *got != *tt.want {
				t.Errorf("NullStringPtr() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func TestNullTimePtr(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		input sql.NullTime
		want  *time.Time
	}{
		{
			name:  "valid time",
			input: sql.NullTime{Time: now, Valid: true},
			want:  &now,
		},
		{
			name:  "invalid time",
			input: sql.NullTime{Time: now, Valid: false},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NullTimePtr(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("NullTimePtr() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Errorf("NullTimePtr() = nil, want %v", *tt.want)
				return
			}
			if !got.Equal(*tt.want) {
				t.Errorf("NullTimePtr() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func TestNullLowerTrimStringPtr(t *testing.T) {
	tests := []struct {
		name  string
		input sql.NullString
		want  *string
	}{
		{
			name:  "valid string with spaces",
			input: sql.NullString{String: "  Hello World  ", Valid: true},
			want:  ptr("hello world"),
		},
		{
			name:  "invalid string",
			input: sql.NullString{String: "hello", Valid: false},
			want:  nil,
		},
		{
			name:  "empty after trim",
			input: sql.NullString{String: "   ", Valid: true},
			want:  nil,
		},
		{
			name:  "already lowercase",
			input: sql.NullString{String: "hello", Valid: true},
			want:  ptr("hello"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NullLowerTrimStringPtr(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("NullLowerTrimStringPtr() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Errorf("NullLowerTrimStringPtr() = nil, want %v", *tt.want)
				return
			}
			if *got != *tt.want {
				t.Errorf("NullLowerTrimStringPtr() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func TestClampText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		want     string
	}{
		{
			name:     "shorter than max",
			input:    "hello",
			maxBytes: 10,
			want:     "hello",
		},
		{
			name:     "exactly max",
			input:    "hello",
			maxBytes: 5,
			want:     "hello",
		},
		{
			name:     "longer than max",
			input:    "hello world",
			maxBytes: 5,
			want:     "hello",
		},
		{
			name:     "empty string",
			input:    "",
			maxBytes: 10,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClampText(tt.input, tt.maxBytes)
			if got != tt.want {
				t.Errorf("ClampText() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}
