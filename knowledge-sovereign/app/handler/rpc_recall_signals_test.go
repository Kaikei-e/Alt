package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestValidateAndBuildRecallSignal(t *testing.T) {
	validSignalID := uuid.New().String()
	validUserID := uuid.New().String()
	occurred := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	pbOccurred := timestamppb.New(occurred)

	tests := []struct {
		name    string
		in      *sovereignv1.RecallSignal
		wantErr string
	}{
		{
			name:    "nil signal returns error",
			in:      nil,
			wantErr: "signal is required",
		},
		{
			name: "invalid signal_id",
			in: &sovereignv1.RecallSignal{
				SignalId: "not-a-uuid",
			},
			wantErr: "invalid signal_id",
		},
		{
			name: "invalid user_id",
			in: &sovereignv1.RecallSignal{
				SignalId: validSignalID,
				UserId:   "not-a-uuid",
			},
			wantErr: "invalid user_id",
		},
		{
			name: "missing occurred_at",
			in: &sovereignv1.RecallSignal{
				SignalId:   validSignalID,
				UserId:     validUserID,
				OccurredAt: nil,
			},
			wantErr: "occurred_at is required",
		},
		{
			name: "valid signal",
			in: &sovereignv1.RecallSignal{
				SignalId:       validSignalID,
				UserId:         validUserID,
				ItemKey:        "article:456",
				SignalType:     "dwell_time",
				SignalStrength: 0.85,
				Payload:        []byte(`{"dwell_ms":12000}`),
				OccurredAt:     pbOccurred,
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAndBuildRecallSignal(tt.in)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, validSignalID, got.SignalID.String())
				assert.Equal(t, validUserID, got.UserID.String())
				assert.Equal(t, "article:456", got.ItemKey)
				assert.Equal(t, "dwell_time", got.SignalType)
				assert.Equal(t, 0.85, got.SignalStrength)
				assert.Equal(t, json.RawMessage(`{"dwell_ms":12000}`), got.Payload)
				assert.True(t, got.OccurredAt.Equal(occurred))
			}
		})
	}
}
