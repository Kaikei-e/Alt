package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildIdempotencyKey(t *testing.T) {
	tests := []struct {
		name     string
		mutType  string
		entityID string
		want     string
	}{
		{"upsert_home_item", "upsert_home_item", "article:abc", "upsert_home_item:article:abc"},
		{"dismiss_home_item", "dismiss_home_item", "article:def", "dismiss_home_item:article:def"},
		{"snooze_candidate", "snooze_candidate", "article:ghi", "snooze_candidate:article:ghi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := BuildIdempotencyKey(tt.mutType, tt.entityID)
			assert.Equal(t, tt.want, key)
		})
	}
}

func TestGetIdempotencyPolicy(t *testing.T) {
	assert.Equal(t, IdempotencyLastWriteWins, GetIdempotencyPolicy("upsert_home_item"))
	assert.Equal(t, IdempotencyLastWriteWins, GetIdempotencyPolicy("upsert_today_digest"))
	assert.Equal(t, IdempotencyLastWriteWins, GetIdempotencyPolicy("upsert_recall_candidate"))
	assert.Equal(t, IdempotencySkipIfApplied, GetIdempotencyPolicy("dismiss_home_item"))
	assert.Equal(t, IdempotencySkipIfApplied, GetIdempotencyPolicy("snooze_candidate"))
	assert.Equal(t, IdempotencySkipIfApplied, GetIdempotencyPolicy("dismiss_candidate"))
}
