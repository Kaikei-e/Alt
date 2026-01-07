// ABOUTME: Tests for domain-level sentinel errors
// ABOUTME: Ensures error values work correctly with errors.Is
package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors_Defined(t *testing.T) {
	// Verify all sentinel errors are defined and non-nil
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrArticleNotFound", ErrArticleNotFound},
		{"ErrArticleContentEmpty", ErrArticleContentEmpty},
		{"ErrContentTooShort", ErrContentTooShort},
		{"ErrJobNotFound", ErrJobNotFound},
		{"ErrInvalidRequest", ErrInvalidRequest},
		{"ErrMissingArticleID", ErrMissingArticleID},
		{"ErrEmptyContent", ErrEmptyContent},
		{"ErrNewsCreatorUnavailable", ErrNewsCreatorUnavailable},
	}

	for _, s := range sentinels {
		t.Run(s.name, func(t *testing.T) {
			if s.err == nil {
				t.Errorf("%s should not be nil", s.name)
			}
			if s.err.Error() == "" {
				t.Errorf("%s should have non-empty message", s.name)
			}
		})
	}
}

func TestSentinelErrors_Is(t *testing.T) {
	// Verify errors.Is works with sentinel errors
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{
			name:   "direct match ErrArticleNotFound",
			err:    ErrArticleNotFound,
			target: ErrArticleNotFound,
			want:   true,
		},
		{
			name:   "wrapped ErrArticleNotFound",
			err:    fmt.Errorf("failed to find article: %w", ErrArticleNotFound),
			target: ErrArticleNotFound,
			want:   true,
		},
		{
			name:   "direct match ErrContentTooShort",
			err:    ErrContentTooShort,
			target: ErrContentTooShort,
			want:   true,
		},
		{
			name:   "wrapped ErrContentTooShort",
			err:    fmt.Errorf("summarization failed: %w", ErrContentTooShort),
			target: ErrContentTooShort,
			want:   true,
		},
		{
			name:   "different errors should not match",
			err:    ErrArticleNotFound,
			target: ErrJobNotFound,
			want:   false,
		},
		{
			name:   "deeply wrapped error",
			err:    fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrMissingArticleID)),
			target: ErrMissingArticleID,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errors.Is(tt.err, tt.target)
			if got != tt.want {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tt.err, tt.target, got, tt.want)
			}
		})
	}
}

func TestSentinelErrors_UniqueMessages(t *testing.T) {
	// Verify each sentinel has a unique message (no copy-paste errors)
	sentinels := []error{
		ErrArticleNotFound,
		ErrArticleContentEmpty,
		ErrContentTooShort,
		ErrJobNotFound,
		ErrInvalidRequest,
		ErrMissingArticleID,
		ErrEmptyContent,
		ErrNewsCreatorUnavailable,
	}

	messages := make(map[string]bool)
	for _, err := range sentinels {
		msg := err.Error()
		if messages[msg] {
			t.Errorf("duplicate error message found: %q", msg)
		}
		messages[msg] = true
	}
}
