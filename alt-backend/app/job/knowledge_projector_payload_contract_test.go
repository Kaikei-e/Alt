package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArticleCreatedPayloadContract pins the wire contract between the producer
// (alt-backend/app/driver/mqhub_connect.ArticleCreatedPayload, json:"url") and
// the projector consumer in this package. It marshals JSON in the canonical
// producer wire form and asserts the projected KnowledgeHomeItem carries the
// link through to the read model. The bug this防波堤 catches is silent JSON-tag
// drift between the two structs — see PM-2026-040 for the same class of bug
// across the JSONB-NULL default boundary.
func TestArticleCreatedPayloadContract_LinkRoundTrips(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()

	// Canonical producer wire form. Built as raw map so the test does not
	// depend on the consumer struct's tag (which is exactly what we are
	// validating). Mirrors mqhub_connect.ArticleCreatedPayload field tags.
	payload, err := json.Marshal(map[string]any{
		"article_id":   articleID.String(),
		"user_id":      tenantID.String(),
		"feed_id":      uuid.New().String(),
		"title":        "Contract Article",
		"url":          "https://example.com/contract-article",
		"published_at": "2026-04-28T10:00:00Z",
	})
	require.NoError(t, err)

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{
				EventID:       uuid.New(),
				EventSeq:      1,
				TenantID:      tenantID,
				EventType:     domain.EventArticleCreated,
				AggregateType: domain.AggregateArticle,
				AggregateID:   articleID.String(),
				Payload:       payload,
			},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	require.NoError(t, fn(context.Background()))

	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t,
		"https://example.com/contract-article",
		homeItemsPort.upserted[0].Link,
		"projector must read the article URL from the canonical producer wire key (json:\"url\"), not a divergent consumer-only key — see knowledge-event-payload-tag-audit-2026-04-28.md",
	)
}
