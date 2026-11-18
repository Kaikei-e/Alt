package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizeOrigin_Validate(t *testing.T) {
	tests := []struct {
		name    string
		origin  SummarizeOrigin
		wantErr error
	}{
		{
			name:    "user origin is valid",
			origin:  SummarizeOriginUser,
			wantErr: nil,
		},
		{
			name:    "system origin is valid",
			origin:  SummarizeOriginSystem,
			wantErr: nil,
		},
		{
			name:    "empty origin is rejected",
			origin:  "",
			wantErr: ErrInvalidSummarizeOrigin,
		},
		{
			name:    "unknown origin is rejected",
			origin:  SummarizeOrigin("other"),
			wantErr: ErrInvalidSummarizeOrigin,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.origin.Validate()
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.False(t, tc.origin.IsValid())
			} else {
				require.NoError(t, err)
				assert.True(t, tc.origin.IsValid())
			}
		})
	}
}
