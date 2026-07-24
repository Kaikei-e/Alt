package handler

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// AppendKnowledgeUserEvent's dedupe relies on a partial unique index
// (`WHERE dedupe_key != ”`) in the driver layer, so an empty dedupe_key
// silently disables deduplication instead of failing. Every other required
// field on this RPC (user_event_id, user_id, tenant_id, occurred_at) is
// already validated at the boundary; dedupe_key must be too.
func TestAppendKnowledgeUserEvent_RejectsEmptyDedupeKey(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	_, err := h.AppendKnowledgeUserEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeUserEventRequest{
		Event: &sovereignv1.KnowledgeUserEvent{
			UserEventId: "11111111-1111-1111-1111-111111111111",
			UserId:      "22222222-2222-2222-2222-222222222222",
			TenantId:    "33333333-3333-3333-3333-333333333333",
			OccurredAt:  timestamppb.Now(),
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

	_, err := h.AppendKnowledgeUserEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeUserEventRequest{
		Event: &sovereignv1.KnowledgeUserEvent{
			UserEventId: "11111111-1111-1111-1111-111111111111",
			UserId:      "22222222-2222-2222-2222-222222222222",
			TenantId:    "33333333-3333-3333-3333-333333333333",
			OccurredAt:  timestamppb.Now(),
			DedupeKey:   "   ",
		},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestAppendKnowledgeUserEvent_AcceptsNonEmptyDedupeKey(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	_, err := h.AppendKnowledgeUserEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeUserEventRequest{
		Event: &sovereignv1.KnowledgeUserEvent{
			UserEventId: "11111111-1111-1111-1111-111111111111",
			UserId:      "22222222-2222-2222-2222-222222222222",
			TenantId:    "33333333-3333-3333-3333-333333333333",
			OccurredAt:  timestamppb.Now(),
			DedupeKey:   "recall_snooze:user:item:123",
		},
	}))
	require.NoError(t, err)
	assert.Equal(t, "AppendKnowledgeUserEvent", repo.lastMethod)
}
