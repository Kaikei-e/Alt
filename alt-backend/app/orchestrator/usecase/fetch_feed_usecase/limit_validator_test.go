package fetch_feed_usecase

import (
	"testing"
)

func TestValidateCursorLimit(t *testing.T) {
	tests := []struct {
		name        string
		limit       int
		expectError bool
		errMsg      string
	}{
		{
			name:        "limit 0 returns error",
			limit:       0,
			expectError: true,
			errMsg:      "limit must be greater than 0",
		},
		{
			name:        "negative limit returns error",
			limit:       -1,
			expectError: true,
			errMsg:      "limit must be greater than 0",
		},
		{
			name:        "limit 1 is valid",
			limit:       1,
			expectError: false,
		},
		{
			name:        "limit 50 is valid",
			limit:       50,
			expectError: false,
		},
		{
			name:        "limit 100 is valid",
			limit:       100,
			expectError: false,
		},
		{
			name:        "limit 101 returns error",
			limit:       101,
			expectError: true,
			errMsg:      "limit cannot exceed 100",
		},
		{
			name:        "large limit returns error",
			limit:       1000,
			expectError: true,
			errMsg:      "limit cannot exceed 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCursorLimit(tt.limit)
			if tt.expectError {
				if err == nil {
					t.Fatalf("validateCursorLimit(%d) expected error, got nil", tt.limit)
				}
				if err.Error() != tt.errMsg {
					t.Errorf("validateCursorLimit(%d) error = %q, want %q", tt.limit, err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateCursorLimit(%d) unexpected error: %v", tt.limit, err)
				}
			}
		})
	}
}
