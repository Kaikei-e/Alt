package job

import (
	"encoding/json"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTodayEntranceTriggerHour_Pure(t *testing.T) {
	tokyo := time.FixedZone("JST", 9*60*60)

	tests := []struct {
		name       string
		t          time.Time
		targetHour int
		want       bool
	}{
		{
			name:       "exact 23:00 UTC matches trigger hour 23",
			t:          time.Date(2026, 9, 25, 23, 0, 0, 0, time.UTC),
			targetHour: 23,
			want:       true,
		},
		{
			name:       "22:59:59 UTC does not match trigger hour 23",
			t:          time.Date(2026, 9, 25, 22, 59, 59, 0, time.UTC),
			targetHour: 23,
			want:       false,
		},
		{
			name:       "23:59:59 UTC matches trigger hour 23",
			t:          time.Date(2026, 9, 25, 23, 59, 59, 0, time.UTC),
			targetHour: 23,
			want:       true,
		},
		{
			name:       "00:00 UTC does not match trigger hour 23",
			t:          time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
			targetHour: 23,
			want:       false,
		},
		{
			name:       "08:00 JST next day matches 23:00 UTC trigger",
			t:          time.Date(2026, 9, 26, 8, 0, 0, 0, tokyo),
			targetHour: 23,
			want:       true,
		},
		{
			name:       "23:00 JST does not match 23:00 UTC trigger",
			t:          time.Date(2026, 9, 25, 23, 0, 0, 0, tokyo),
			targetHour: 23,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTodayEntranceTriggerHour(tt.t, tt.targetHour)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildTodayEntranceEnqueue(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 9, 25, 23, 15, 0, 0, time.UTC)
	ttl := 24 * time.Hour

	tests := []struct {
		name      string
		userID    uuid.UUID
		count     int
		now       time.Time
		ttl       time.Duration
		wantErr   bool
		wantCount int
	}{
		{
			name:      "positive count builds valid enqueue model",
			userID:    userID,
			count:     5,
			now:       now,
			ttl:       ttl,
			wantErr:   false,
			wantCount: 5,
		},
		{
			name:      "zero count builds enqueue model with zero",
			userID:    userID,
			count:     0,
			now:       now,
			ttl:       ttl,
			wantErr:   false,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildTodayEntranceEnqueue(tt.userID, tt.count, tt.now, tt.ttl)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "digest:"+tt.userID.String()+":2026-09-25", got.DedupeKey)
			assert.Equal(t, tt.userID.String(), got.UserID)
			assert.Equal(t, domain.NotificationKindTodayEntranceReady, got.Kind)
			assert.Equal(t, tt.now, got.OccurredAt)
			assert.Equal(t, tt.now.Add(tt.ttl), got.ExpiresAt)

			var payload struct {
				Kind  string `json:"kind"`
				URL   string `json:"url"`
				Count int    `json:"count"`
			}
			require.NoError(t, json.Unmarshal(got.Payload, &payload))
			assert.Equal(t, domain.NotificationKindTodayEntranceReady, payload.Kind)
			assert.Equal(t, "/home", payload.URL)
			assert.Equal(t, tt.wantCount, payload.Count)
		})
	}
}
