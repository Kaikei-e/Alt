package job

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRetryBudgetExhausted(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		maxAttempts int
		want        bool
	}{
		{
			name:        "attempt below limit is not exhausted",
			attempt:     1,
			maxAttempts: 24,
			want:        false,
		},
		{
			name:        "attempt right below limit is not exhausted",
			attempt:     23,
			maxAttempts: 24,
			want:        false,
		},
		{
			name:        "attempt equal to limit is exhausted",
			attempt:     24,
			maxAttempts: 24,
			want:        true,
		},
		{
			name:        "attempt exceeding limit is exhausted",
			attempt:     25,
			maxAttempts: 24,
			want:        true,
		},
		{
			name:        "zero attempts with positive limit is not exhausted",
			attempt:     0,
			maxAttempts: 5,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryBudgetExhausted(tt.attempt, tt.maxAttempts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOutboxRetryTracker_Lifecycle(t *testing.T) {
	tracker := newOutboxRetryTracker()
	id := "event-123"

	assert.False(t, tracker.ragUpsertDone(id))
	assert.Equal(t, 1, tracker.recordFailure(id))
	assert.Equal(t, 2, tracker.recordFailure(id))

	tracker.markRagUpserted(id)
	assert.True(t, tracker.ragUpsertDone(id))

	// After markRagUpserted, attempt count is reset so failure count starts from 1 again
	assert.Equal(t, 1, tracker.recordFailure(id))

	tracker.clear(id)
	assert.False(t, tracker.ragUpsertDone(id))
	assert.Equal(t, 1, tracker.recordFailure(id))
}
