package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestValidateAndBuildKnowledgeEvent(t *testing.T) {
	validEventID := uuid.New().String()
	validTenantID := uuid.New().String()
	validUserID := uuid.New().String()
	validCorrelationID := uuid.New().String()
	validCausationID := uuid.New().String()
	occurred := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	pbOccurred := timestamppb.New(occurred)

	tests := []struct {
		name    string
		in      *sovereignv1.KnowledgeEvent
		wantErr string
	}{
		{
			name:    "nil event returns error",
			in:      nil,
			wantErr: "event is required",
		},
		{
			name: "invalid event_id",
			in: &sovereignv1.KnowledgeEvent{
				EventId:  "not-a-uuid",
				TenantId: validTenantID,
			},
			wantErr: "invalid event_id",
		},
		{
			name: "invalid tenant_id",
			in: &sovereignv1.KnowledgeEvent{
				EventId:  validEventID,
				TenantId: "not-a-uuid",
			},
			wantErr: "invalid tenant_id",
		},
		{
			name: "invalid user_id",
			in: &sovereignv1.KnowledgeEvent{
				EventId:  validEventID,
				TenantId: validTenantID,
				UserId:   "not-a-uuid",
			},
			wantErr: "invalid user_id",
		},
		{
			name: "invalid correlation_id",
			in: &sovereignv1.KnowledgeEvent{
				EventId:       validEventID,
				TenantId:      validTenantID,
				CorrelationId: "not-a-uuid",
			},
			wantErr: "invalid correlation_id",
		},
		{
			name: "invalid causation_id",
			in: &sovereignv1.KnowledgeEvent{
				EventId:     validEventID,
				TenantId:    validTenantID,
				CausationId: "not-a-uuid",
			},
			wantErr: "invalid causation_id",
		},
		{
			name: "missing occurred_at",
			in: &sovereignv1.KnowledgeEvent{
				EventId:    validEventID,
				TenantId:   validTenantID,
				UserId:     validUserID,
				OccurredAt: nil,
			},
			wantErr: "occurred_at is required",
		},
		{
			name: "valid full event",
			in: &sovereignv1.KnowledgeEvent{
				EventId:       validEventID,
				TenantId:      validTenantID,
				UserId:        validUserID,
				CorrelationId: validCorrelationID,
				CausationId:   validCausationID,
				ActorType:     "user",
				ActorId:       "actor-1",
				EventType:     "ArticleRead",
				AggregateType: "article",
				AggregateId:   "art-123",
				DedupeKey:     "dedupe-1",
				Payload:       []byte(`{"key":"value"}`),
				OccurredAt:    pbOccurred,
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAndBuildKnowledgeEvent(tt.in)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, validEventID, got.EventID.String())
				assert.Equal(t, validTenantID, got.TenantID.String())
				require.NotNil(t, got.UserID)
				assert.Equal(t, validUserID, got.UserID.String())
				require.NotNil(t, got.CorrelationID)
				assert.Equal(t, validCorrelationID, got.CorrelationID.String())
				require.NotNil(t, got.CausationID)
				assert.Equal(t, validCausationID, got.CausationID.String())
				assert.Equal(t, "ArticleRead", got.EventType)
				assert.True(t, got.OccurredAt.Equal(occurred))
			}
		})
	}
}

func TestValidateAndBuildKnowledgeUserEvent(t *testing.T) {
	validEventID := uuid.New().String()
	validTenantID := uuid.New().String()
	validUserID := uuid.New().String()
	occurred := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	pbOccurred := timestamppb.New(occurred)

	tests := []struct {
		name    string
		in      *sovereignv1.KnowledgeUserEvent
		wantErr string
	}{
		{
			name:    "nil event returns error",
			in:      nil,
			wantErr: "event is required",
		},
		{
			name: "invalid user_event_id",
			in: &sovereignv1.KnowledgeUserEvent{
				UserEventId: "not-a-uuid",
			},
			wantErr: "invalid user_event_id",
		},
		{
			name: "invalid user_id",
			in: &sovereignv1.KnowledgeUserEvent{
				UserEventId: validEventID,
				UserId:      "not-a-uuid",
			},
			wantErr: "invalid user_id",
		},
		{
			name: "invalid tenant_id",
			in: &sovereignv1.KnowledgeUserEvent{
				UserEventId: validEventID,
				UserId:      validUserID,
				TenantId:    "not-a-uuid",
			},
			wantErr: "invalid tenant_id",
		},
		{
			name: "missing occurred_at",
			in: &sovereignv1.KnowledgeUserEvent{
				UserEventId: validEventID,
				UserId:      validUserID,
				TenantId:    validTenantID,
				OccurredAt:  nil,
			},
			wantErr: "occurred_at is required",
		},
		{
			name: "empty dedupe_key",
			in: &sovereignv1.KnowledgeUserEvent{
				UserEventId: validEventID,
				UserId:      validUserID,
				TenantId:    validTenantID,
				OccurredAt:  pbOccurred,
				DedupeKey:   "",
			},
			wantErr: "dedupe_key is required",
		},
		{
			name: "whitespace dedupe_key",
			in: &sovereignv1.KnowledgeUserEvent{
				UserEventId: validEventID,
				UserId:      validUserID,
				TenantId:    validTenantID,
				OccurredAt:  pbOccurred,
				DedupeKey:   "   ",
			},
			wantErr: "dedupe_key is required",
		},
		{
			name: "valid user event",
			in: &sovereignv1.KnowledgeUserEvent{
				UserEventId: validEventID,
				UserId:      validUserID,
				TenantId:    validTenantID,
				OccurredAt:  pbOccurred,
				DedupeKey:   "item:123:action",
				EventType:   "ItemBookmarked",
				ItemKey:     "item:123",
				Payload:     []byte("{}"),
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAndBuildKnowledgeUserEvent(tt.in)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, validEventID, got.UserEventID.String())
				assert.Equal(t, validUserID, got.UserID.String())
				assert.Equal(t, validTenantID, got.TenantID.String())
				assert.Equal(t, "item:123:action", got.DedupeKey)
				assert.Equal(t, "ItemBookmarked", got.EventType)
				assert.Equal(t, "item:123", got.ItemKey)
				assert.True(t, got.OccurredAt.Equal(occurred))
			}
		})
	}
}

func TestEventToProto(t *testing.T) {
	eventID := uuid.New()
	tenantID := uuid.New()
	userID := uuid.New()
	corrID := uuid.New()
	causID := uuid.New()
	occurred := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	event := sovereign_db.KnowledgeEvent{
		EventID:       eventID,
		EventSeq:      42,
		ActorType:     "agent",
		ActorID:       "agent-007",
		EventType:     "SummaryGenerated",
		AggregateType: "summary",
		AggregateID:   "sum-1",
		DedupeKey:     "dedupe-42",
		Payload:       json.RawMessage(`{"summary":"text"}`),
		TenantID:      tenantID,
		UserID:        &userID,
		CorrelationID: &corrID,
		CausationID:   &causID,
		OccurredAt:    occurred,
	}

	pb := eventToProto(event)
	require.NotNil(t, pb)
	assert.Equal(t, eventID.String(), pb.EventId)
	assert.Equal(t, int64(42), pb.EventSeq)
	assert.Equal(t, tenantID.String(), pb.TenantId)
	assert.Equal(t, userID.String(), pb.UserId)
	assert.Equal(t, corrID.String(), pb.CorrelationId)
	assert.Equal(t, causID.String(), pb.CausationId)
	assert.Equal(t, "agent", pb.ActorType)
	assert.Equal(t, "agent-007", pb.ActorId)
	assert.Equal(t, "SummaryGenerated", pb.EventType)
	assert.Equal(t, "summary", pb.AggregateType)
	assert.Equal(t, "sum-1", pb.AggregateId)
	assert.Equal(t, "dedupe-42", pb.DedupeKey)
	assert.JSONEq(t, `{"summary":"text"}`, string(pb.Payload))
	assert.True(t, pb.OccurredAt.AsTime().Equal(occurred))
}

// TestAppendKnowledgeEvent_InvalidEventID_ReturnsInvalidArgument pins the
// same fix for the write path: a malformed event_id must never reach
// AppendKnowledgeEvent (which would otherwise write uuid.Nil into
// knowledge_events).
func TestAppendKnowledgeEvent_InvalidEventID_ReturnsInvalidArgument(t *testing.T) {
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	_, err := client.AppendKnowledgeEvent(context.Background(),
		connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
			Event: &sovereignv1.KnowledgeEvent{
				EventId:   "not-a-uuid",
				TenantId:  uuid.New().String(),
				EventType: "ArticleCreated",
			},
		}))

	require.Error(t, err, "malformed event_id must be rejected before it can be written to knowledge_events")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestAppendKnowledgeEvent_InvalidCorrelationID_ReturnsInvalidArgument
// covers the optional-UUID-pointer path (parseUUIDPtrField): a non-empty
// but malformed correlation_id must also error, not silently become nil.
func TestAppendKnowledgeEvent_InvalidCorrelationID_ReturnsInvalidArgument(t *testing.T) {
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	_, err := client.AppendKnowledgeEvent(context.Background(),
		connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
			Event: &sovereignv1.KnowledgeEvent{
				EventId:       uuid.New().String(),
				TenantId:      uuid.New().String(),
				EventType:     "ArticleCreated",
				CorrelationId: "not-a-uuid",
			},
		}))

	require.Error(t, err, "malformed correlation_id must be rejected, not silently dropped to nil")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// AppendKnowledgeUserEvent's dedupe relies on a partial unique index
// (`WHERE dedupe_key != ”`) in the driver layer, so an empty dedupe_key
// silently disables deduplication instead of failing. Every other required
// field on this RPC (user_event_id, user_id, tenant_id, occurred_at) is
// already validated at the boundary; dedupe_key must be too.
func TestAppendKnowledgeUserEvent_RejectsEmptyDedupeKey(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)
	fixedTime := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	_, err := h.AppendKnowledgeUserEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeUserEventRequest{
		Event: &sovereignv1.KnowledgeUserEvent{
			UserEventId: "11111111-1111-1111-1111-111111111111",
			UserId:      "22222222-2222-2222-2222-222222222222",
			TenantId:    "33333333-3333-3333-3333-333333333333",
			OccurredAt:  timestamppb.New(fixedTime),
			DedupeKey:   "",
		},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Empty(t, repo.lastMethod, "an empty dedupe_key must never reach the repository")
}

func TestAppendKnowledgeUserEvent_RejectsWhitespaceOnlyDedupeKey(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)
	fixedTime := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	_, err := h.AppendKnowledgeUserEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeUserEventRequest{
		Event: &sovereignv1.KnowledgeUserEvent{
			UserEventId: "11111111-1111-1111-1111-111111111111",
			UserId:      "22222222-2222-2222-2222-222222222222",
			TenantId:    "33333333-3333-3333-3333-333333333333",
			OccurredAt:  timestamppb.New(fixedTime),
			DedupeKey:   "   ",
		},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestAppendKnowledgeUserEvent_AcceptsNonEmptyDedupeKey(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)
	fixedTime := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	_, err := h.AppendKnowledgeUserEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeUserEventRequest{
		Event: &sovereignv1.KnowledgeUserEvent{
			UserEventId: "11111111-1111-1111-1111-111111111111",
			UserId:      "22222222-2222-2222-2222-222222222222",
			TenantId:    "33333333-3333-3333-3333-333333333333",
			OccurredAt:  timestamppb.New(fixedTime),
			DedupeKey:   "recall_snooze:user:item:123",
		},
	}))
	require.NoError(t, err)
	assert.Equal(t, "AppendKnowledgeUserEvent", repo.lastMethod)
}

func TestAppendKnowledgeEvent_RejectsEmptyOrWhitespaceDedupeKey(t *testing.T) {
	fixedTime := timestamppb.New(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC))

	for _, key := range []string{"", "   ", "\t\n"} {
		t.Run(fmt.Sprintf("key=%q", key), func(t *testing.T) {
			repo := &mockRepo{}
			h := NewSovereignHandler(repo)

			_, err := h.AppendKnowledgeEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
				Event: &sovereignv1.KnowledgeEvent{
					EventId:    uuid.New().String(),
					TenantId:   uuid.New().String(),
					EventType:  "ArticleCreated",
					DedupeKey:  key,
					Payload:    []byte(`{"title":"hello"}`),
					OccurredAt: fixedTime,
				},
			}))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Empty(t, repo.lastMethod)
		})
	}
}

func TestAppendKnowledgeEvent_ValidatesPayloadJSONObjectAndSizeBound(t *testing.T) {
	assert.Equal(t, 64*1024, MaxKnowledgeEventPayloadBytes)
	fixedTime := timestamppb.New(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC))

	invalidPayloads := []struct {
		name    string
		payload []byte
	}{
		{name: "nil payload", payload: nil},
		{name: "empty bytes", payload: []byte("")},
		{name: "whitespace only", payload: []byte("   ")},
		{name: "malformed JSON", payload: []byte(`{"summary":`)},
		{name: "JSON array", payload: []byte(`["a","b"]`)},
		{name: "JSON string", payload: []byte(`"hello"`)},
		{name: "JSON number", payload: []byte(`42`)},
		{name: "JSON null", payload: []byte(`null`)},
		{name: "trailing JSON value", payload: []byte(`{"a":1}{"b":2}`)},
		{name: "invalid UTF-8", payload: []byte{'{', '"', 'k', '"', ':', '"', 0xff, '"', '}'}},
		{
			name:    "exceeds MaxKnowledgeEventPayloadBytes",
			payload: []byte(`{"s":"` + strings.Repeat("a", MaxKnowledgeEventPayloadBytes) + `"}`),
		},
	}

	for _, tc := range invalidPayloads {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepo{}
			h := NewSovereignHandler(repo)

			_, err := h.AppendKnowledgeEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
				Event: &sovereignv1.KnowledgeEvent{
					EventId:    uuid.New().String(),
					TenantId:   uuid.New().String(),
					EventType:  "SummaryVersionCreated",
					DedupeKey:  "SummaryVersionCreated:" + uuid.New().String(),
					Payload:    tc.payload,
					OccurredAt: fixedTime,
				},
			}))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Empty(t, repo.lastMethod)
		})
	}

	t.Run("accepts JSON object at exact MaxKnowledgeEventPayloadBytes boundary", func(t *testing.T) {
		prefix := []byte(`{"s":"`)
		suffix := []byte(`"}`)
		exact := bytes.Join([][]byte{prefix, bytes.Repeat([]byte("a"), MaxKnowledgeEventPayloadBytes-len(prefix)-len(suffix)), suffix}, nil)
		require.Len(t, exact, MaxKnowledgeEventPayloadBytes)

		repo := &mockRepo{}
		h := NewSovereignHandler(repo)

		_, err := h.AppendKnowledgeEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
			Event: &sovereignv1.KnowledgeEvent{
				EventId:    uuid.New().String(),
				TenantId:   uuid.New().String(),
				EventType:  "SummaryVersionCreated",
				DedupeKey:  "SummaryVersionCreated:" + uuid.New().String(),
				Payload:    exact,
				OccurredAt: fixedTime,
			},
		}))
		require.NoError(t, err)
	})
}

func TestAppendKnowledgeEvent_ValidatesTenantAndUserIdentifiers(t *testing.T) {
	fixedTime := timestamppb.New(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC))

	invalidCases := []struct {
		name     string
		tenantID string
		userID   string
	}{
		{name: "empty tenant_id", tenantID: "", userID: ""},
		{name: "whitespace tenant_id", tenantID: "   ", userID: ""},
		{name: "nil UUID tenant_id", tenantID: uuid.Nil.String(), userID: ""},
		{name: "whitespace user_id", tenantID: uuid.New().String(), userID: "   "},
		{name: "nil UUID user_id", tenantID: uuid.New().String(), userID: uuid.Nil.String()},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepo{}
			h := NewSovereignHandler(repo)

			_, err := h.AppendKnowledgeEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
				Event: &sovereignv1.KnowledgeEvent{
					EventId:    uuid.New().String(),
					TenantId:   tc.tenantID,
					UserId:     tc.userID,
					EventType:  "ArticleCreated",
					DedupeKey:  "article-created:" + uuid.New().String(),
					Payload:    []byte(`{}`),
					OccurredAt: fixedTime,
				},
			}))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}

	t.Run("accepts absent user_id for tenant-wide event", func(t *testing.T) {
		repo := &mockRepo{}
		h := NewSovereignHandler(repo)

		_, err := h.AppendKnowledgeEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
			Event: &sovereignv1.KnowledgeEvent{
				EventId:    uuid.New().String(),
				TenantId:   uuid.New().String(),
				UserId:     "",
				EventType:  "ArticleCreated",
				DedupeKey:  "article-created:" + uuid.New().String(),
				Payload:    []byte(`{}`),
				OccurredAt: fixedTime,
			},
		}))
		require.NoError(t, err)
	})
}

func TestAppendKnowledgeEvent_CrossTenantDedupeCollision_ReturnsPermissionDenied(t *testing.T) {
	fixedTime := timestamppb.New(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC))
	repo := &mockRepo{
		returnErr: fmt.Errorf("dedupe check: %w", sovereign_db.ErrDedupeKeyTenantMismatch),
	}
	h := NewSovereignHandler(repo)

	_, err := h.AppendKnowledgeEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
		Event: &sovereignv1.KnowledgeEvent{
			EventId:    uuid.New().String(),
			TenantId:   uuid.New().String(),
			EventType:  "ArticleCreated",
			DedupeKey:  "article-created:shared-key",
			Payload:    []byte(`{}`),
			OccurredAt: fixedTime,
		},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}
