package rest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArticleLimit(t *testing.T) {
	tests := []struct {
		name     string
		limitStr string
		want     int
		wantErr  bool
	}{
		{
			name:     "empty string defaults to 20",
			limitStr: "",
			want:     20,
			wantErr:  false,
		},
		{
			name:     "valid positive limit within bound",
			limitStr: "15",
			want:     15,
			wantErr:  false,
		},
		{
			name:     "valid max articles page size",
			limitStr: "99",
			want:     99,
			wantErr:  false,
		},
		{
			name:     "limit above max clamped to maxArticlesPageSize (99)",
			limitStr: "100",
			want:     99,
			wantErr:  false,
		},
		{
			name:     "limit far above max clamped to maxArticlesPageSize (99)",
			limitStr: "1000",
			want:     99,
			wantErr:  false,
		},
		{
			name:     "zero limit returns error",
			limitStr: "0",
			want:     0,
			wantErr:  true,
		},
		{
			name:     "negative limit returns error",
			limitStr: "-5",
			want:     0,
			wantErr:  true,
		},
		{
			name:     "non-numeric string returns error",
			limitStr: "invalid",
			want:     0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArticleLimit(tt.limitStr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseArticleCursor(t *testing.T) {
	refTime := time.Date(2026, time.September, 25, 7, 0, 0, 0, time.UTC)
	refTimeStr := refTime.Format(time.RFC3339)

	tests := []struct {
		name      string
		cursorStr string
		wantTime  *time.Time
		wantErr   bool
	}{
		{
			name:      "empty string returns nil cursor without error",
			cursorStr: "",
			wantTime:  nil,
			wantErr:   false,
		},
		{
			name:      "valid RFC3339 timestamp",
			cursorStr: refTimeStr,
			wantTime:  &refTime,
			wantErr:   false,
		},
		{
			name:      "invalid date format returns error",
			cursorStr: "2026-09-25",
			wantTime:  nil,
			wantErr:   true,
		},
		{
			name:      "arbitrary invalid text returns error",
			cursorStr: "not-a-date",
			wantTime:  nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArticleCursor(tt.cursorStr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.wantTime == nil {
					assert.Nil(t, got)
				} else {
					require.NotNil(t, got)
					assert.True(t, got.Equal(*tt.wantTime))
				}
			}
		})
	}
}
