package sovereign_db

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultBackfillJobKind(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string defaults to articles",
			in:   "",
			want: "articles",
		},
		{
			name: "existing kind preserved",
			in:   "tags",
			want: "tags",
		},
		{
			name: "explicit articles preserved",
			in:   "articles",
			want: "articles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultBackfillJobKind(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultRecallSignalPayload(t *testing.T) {
	tests := []struct {
		name string
		in   json.RawMessage
		want string
	}{
		{
			name: "nil payload defaults to empty object",
			in:   nil,
			want: "{}",
		},
		{
			name: "empty byte slice defaults to empty object",
			in:   json.RawMessage([]byte{}),
			want: "{}",
		},
		{
			name: "valid JSON preserved",
			in:   json.RawMessage(`{"click":true}`),
			want: `{"click":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultRecallSignalPayload(tt.in)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestBuildCreateBackfillJobArgs(t *testing.T) {
	jobID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	t0 := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)

	job := BackfillJob{
		JobID:             jobID,
		Status:            "queued",
		Kind:              "",
		ProjectionVersion: 1,
		CreatedAt:         t0,
		UpdatedAt:         t0,
	}

	args := buildCreateBackfillJobArgs(job)
	require.Len(t, args, 14)
	assert.Equal(t, jobID, args[0])
	assert.Equal(t, "queued", args[1])
	assert.Equal(t, "articles", args[2])
	assert.Equal(t, 1, args[3])
}

func TestBuildUpdateBackfillJobArgs(t *testing.T) {
	jobID := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	t0 := time.Date(2026, 9, 25, 16, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 9, 25, 17, 0, 0, 0, time.UTC)

	job := BackfillJob{
		JobID:           jobID,
		Status:          "completed",
		TotalEvents:     100,
		ProcessedEvents: 100,
		StartedAt:       &t0,
		CompletedAt:     &t1,
	}

	args := buildUpdateBackfillJobArgs(job)
	require.Len(t, args, 10)
	assert.Equal(t, jobID, args[0])
	assert.Equal(t, "completed", args[1])
	assert.Equal(t, 100, args[5])
	assert.Equal(t, 100, args[6])
	assert.Equal(t, &t0, args[8])
	assert.Equal(t, &t1, args[9])
}

func TestBuildAppendRecallSignalArgs(t *testing.T) {
	sigID := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	userID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	t0 := time.Date(2026, 9, 25, 18, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		payload json.RawMessage
		want    string
	}{
		{
			name:    "nil payload defaults to empty JSON",
			payload: nil,
			want:    "{}",
		},
		{
			name:    "non-nil payload preserved",
			payload: json.RawMessage(`{"score":0.9}`),
			want:    `{"score":0.9}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := RecallSignal{
				SignalID:       sigID,
				UserID:         userID,
				ItemKey:        "article:123",
				SignalType:     "open",
				SignalStrength: 0.9,
				OccurredAt:     t0,
				Payload:        tt.payload,
			}
			args := buildAppendRecallSignalArgs(s)
			require.Len(t, args, 7)
			assert.Equal(t, sigID, args[0])
			assert.Equal(t, userID, args[1])
			assert.Equal(t, "article:123", args[2])
			assert.Equal(t, "open", args[3])
			assert.Equal(t, 0.9, args[4])
			assert.Equal(t, t0, args[5])
			assert.Equal(t, tt.want, string(args[6].(json.RawMessage)))
		})
	}
}

func TestScanBackfillJob_Error(t *testing.T) {
	mock := &mockRow{scanFunc: func(_ ...interface{}) error {
		return errors.New("scan failed")
	}}

	_, err := scanBackfillJob(mock)
	require.Error(t, err)
}

func TestScanRecallSignal_Error(t *testing.T) {
	mock := &mockRow{scanFunc: func(_ ...interface{}) error {
		return errors.New("scan failed")
	}}

	_, err := scanRecallSignal(mock)
	require.Error(t, err)
}
