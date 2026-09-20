package driver

import (
	"testing"
	"time"
)

func TestContainsCJK_Japanese(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"イラン 石油", true},
		{"石油危機 原因", true},
		{"量子コンピュータ 実用化", true},
		{"Iran oil crisis", false},
		{"hello world", false},
		{"", false},
		{"AI関連ニュース", true},
		{"GPT-4o と Claude", true},        // と is Hiragana → CJK
		{"GPT-4oとClaude 3.5の違いは？", true}, // contains と, の, は (Hiragana)
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := containsCJK(tt.input)
			if got != tt.expected {
				t.Errorf("containsCJK(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildUserFilter(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		expected string
	}{
		{
			name:     "valid userID",
			userID:   "user-123",
			expected: `user_id = "user-123"`,
		},
		{
			name:     "escaping quotes and backslashes",
			userID:   `user"1\2`,
			expected: `user_id = "user\"1\\2"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildUserFilter(tt.userID)
			if result != tt.expected {
				t.Errorf("BuildUserFilter(%q) = %q, want %q", tt.userID, result, tt.expected)
			}
		})
	}

	t.Run("empty userID panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("BuildUserFilter with empty string should panic")
			}
		}()
		_ = BuildUserFilter("")
	})

	t.Run("whitespace userID panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("BuildUserFilter with whitespace string should panic")
			}
		}()
		_ = BuildUserFilter("   ")
	})
}

func TestMeilisearchDriver_SearchByUserID_RejectsEmptyUserID(t *testing.T) {
	driver := &MeilisearchDriver{}

	_, err := driver.SearchByUserID(t.Context(), "query", "", 10)
	if err == nil {
		t.Error("SearchByUserID with empty userID should return error")
	}

	_, err = driver.SearchByUserID(t.Context(), "query", "   ", 10)
	if err == nil {
		t.Error("SearchByUserID with whitespace userID should return error")
	}
}

func TestMeilisearchDriver_SearchByUserIDWithPagination_RejectsEmptyUserID(t *testing.T) {
	driver := &MeilisearchDriver{}

	_, _, err := driver.SearchByUserIDWithPagination(t.Context(), "query", "", 0, 10)
	if err == nil {
		t.Error("SearchByUserIDWithPagination with empty userID should return error")
	}

	_, _, err = driver.SearchByUserIDWithPagination(t.Context(), "query", "   ", 0, 10)
	if err == nil {
		t.Error("SearchByUserIDWithPagination with whitespace userID should return error")
	}
}

func TestBuildUserDateFilter(t *testing.T) {
	after := time.Unix(1700000000, 0)
	before := time.Unix(1700003600, 0)

	tests := []struct {
		name     string
		userID   string
		after    *time.Time
		before   *time.Time
		expected string
	}{
		{
			name:     "both bounds present",
			userID:   "u1",
			after:    &after,
			before:   &before,
			expected: `user_id = "u1" AND published_at >= 1700000000 AND published_at <= 1700003600`,
		},
		{
			name:     "only published_after",
			userID:   "u1",
			after:    &after,
			before:   nil,
			expected: `user_id = "u1" AND published_at >= 1700000000`,
		},
		{
			name:     "only published_before",
			userID:   "u1",
			after:    nil,
			before:   &before,
			expected: `user_id = "u1" AND published_at <= 1700003600`,
		},
		{
			name:     "neither bound present",
			userID:   "u1",
			after:    nil,
			before:   nil,
			expected: `user_id = "u1"`,
		},
		{
			name:     "escapes user ID with quotes",
			userID:   `user"1`,
			after:    &after,
			before:   &before,
			expected: `user_id = "user\"1" AND published_at >= 1700000000 AND published_at <= 1700003600`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildUserDateFilter(tt.userID, tt.after, tt.before)
			if got != tt.expected {
				t.Errorf("BuildUserDateFilter() = %q, want %q", got, tt.expected)
			}
		})
	}

	t.Run("empty userID panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("BuildUserDateFilter with empty userID should panic")
			}
		}()
		_ = BuildUserDateFilter("", &after, &before)
	})

	t.Run("whitespace userID panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("BuildUserDateFilter with whitespace userID should panic")
			}
		}()
		_ = BuildUserDateFilter("   ", &after, &before)
	})
}

func TestMeilisearchDriver_SearchByUserIDWithDateFilter_RejectsEmptyUserID(t *testing.T) {
	driver := &MeilisearchDriver{}
	after := time.Unix(1700000000, 0)

	_, err := driver.SearchByUserIDWithDateFilter(t.Context(), "query", "", &after, nil, 10)
	if err == nil {
		t.Error("SearchByUserIDWithDateFilter with empty userID should return error")
	}

	_, err = driver.SearchByUserIDWithDateFilter(t.Context(), "query", "   ", &after, nil, 10)
	if err == nil {
		t.Error("SearchByUserIDWithDateFilter with whitespace userID should return error")
	}
}
