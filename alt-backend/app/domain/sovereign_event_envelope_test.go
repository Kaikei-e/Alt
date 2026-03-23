package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSovereignEventEnvelope_Fields(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"article_id": "123"})
	env := SovereignEventEnvelope{
		EventID:        uuid.New(),
		Source:         "pre-processor",
		EventType:      EventArticleCreated,
		OccurredAt:     time.Now(),
		IdempotencyKey: "ArticleCreated:article:123",
		AggregateType:  AggregateArticle,
		AggregateID:    "article:123",
		TenantID:       uuid.New(),
		ActorType:      ActorService,
		ActorID:        "pre-processor",
		Payload:        payload,
		SchemaVersion:  SovereignEnvelopeSchemaV1,
	}
	assert.Equal(t, "pre-processor", env.Source)
	assert.Equal(t, EventArticleCreated, env.EventType)
	assert.Equal(t, SovereignEnvelopeSchemaV1, env.SchemaVersion)
}

func TestSovereignEventEnvelope_ValidSources(t *testing.T) {
	for _, src := range ValidEnvelopeSources {
		assert.NotEmpty(t, src)
	}
	assert.Contains(t, ValidEnvelopeSources, "pre-processor")
	assert.Contains(t, ValidEnvelopeSources, "tag-generator")
	assert.Contains(t, ValidEnvelopeSources, "recap-worker")
	assert.Contains(t, ValidEnvelopeSources, "alt-backend")
	assert.Contains(t, ValidEnvelopeSources, "user")
}

func TestSovereignEventEnvelope_FromKnowledgeEvent(t *testing.T) {
	userID := uuid.New()
	event := KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      uuid.New(),
		UserID:        &userID,
		ActorType:     ActorSystem,
		ActorID:       "pre-processor",
		EventType:     EventArticleCreated,
		AggregateType: AggregateArticle,
		AggregateID:   "article:456",
		DedupeKey:     "ArticleCreated:article:456",
		Payload:       json.RawMessage(`{}`),
	}
	env := NewSovereignEventEnvelopeFromEvent(event, "pre-processor")
	assert.Equal(t, event.EventID, env.EventID)
	assert.Equal(t, "pre-processor", env.Source)
	assert.Equal(t, event.DedupeKey, env.IdempotencyKey)
	assert.Equal(t, SovereignEnvelopeSchemaV1, env.SchemaVersion)
	assert.Equal(t, event.EventType, env.EventType)
	assert.Equal(t, event.TenantID, env.TenantID)
}

func TestSovereignEventEnvelope_RoundTrip(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"key": "value"})
	original := SovereignEventEnvelope{
		EventID:        uuid.New(),
		Source:         "tag-generator",
		EventType:      EventTagSetVersionCreated,
		OccurredAt:     time.Now().Truncate(time.Millisecond),
		IdempotencyKey: "test-key",
		AggregateType:  AggregateArticle,
		AggregateID:    "article:789",
		TenantID:       uuid.New(),
		ActorType:      ActorService,
		ActorID:        "tag-generator",
		Payload:        payload,
		SchemaVersion:  SovereignEnvelopeSchemaV1,
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)
	var decoded SovereignEventEnvelope
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.EventID, decoded.EventID)
	assert.Equal(t, original.Source, decoded.Source)
	assert.Equal(t, original.IdempotencyKey, decoded.IdempotencyKey)
}
