package augur

import (
	"io"
	"log/slog"
	"testing"

	augurv2 "alt/gen/proto/alt/augur/v2"

	"github.com/stretchr/testify/assert"
)

// sanitizeMetaEvent must preserve conversation_id end-to-end so the
// frontend can keep sending the same id on subsequent turns. Stripping it
// (the original behaviour) caused every turn to spawn a brand-new
// conversation row, fragmenting the user's history.
func TestSanitizeMetaEvent_PreservesConversationID(t *testing.T) {
	h := NewHandler(nil, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	in := &augurv2.StreamChatResponse{
		Kind: "meta",
		Payload: &augurv2.StreamChatResponse_Meta{
			Meta: &augurv2.MetaPayload{
				ConversationId: "11111111-2222-3333-4444-555555555555",
				Citations: []*augurv2.Citation{
					{Url: "https://example.com", Title: "Example"},
				},
			},
		},
	}

	out := h.sanitizeMetaEvent(in)
	meta := out.GetMeta()
	if assert.NotNil(t, meta, "sanitized event must still carry MetaPayload") {
		assert.Equal(t, "11111111-2222-3333-4444-555555555555", meta.ConversationId,
			"conversation_id must round-trip through sanitization")
		assert.Len(t, meta.Citations, 1, "citations array must be preserved")
		assert.Equal(t, "https://example.com", meta.Citations[0].Url)
	}
}
