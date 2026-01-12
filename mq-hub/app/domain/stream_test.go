package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreamKey_Constants(t *testing.T) {
	// Verify stream key constants are defined correctly
	assert.Equal(t, StreamKey("alt:events:articles"), StreamKeyArticles)
	assert.Equal(t, StreamKey("alt:events:summaries"), StreamKeySummaries)
	assert.Equal(t, StreamKey("alt:events:tags"), StreamKeyTags)
	assert.Equal(t, StreamKey("alt:events:index"), StreamKeyIndex)
}

func TestConsumerGroup_Constants(t *testing.T) {
	// Verify consumer group constants are defined correctly
	assert.Equal(t, ConsumerGroup("pre-processor-group"), ConsumerGroupPreProcessor)
	assert.Equal(t, ConsumerGroup("tag-generator-group"), ConsumerGroupTagGenerator)
	assert.Equal(t, ConsumerGroup("search-indexer-group"), ConsumerGroupSearchIndexer)
}

func TestStreamKey_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		key   StreamKey
		valid bool
	}{
		{"valid articles stream", StreamKeyArticles, true},
		{"valid summaries stream", StreamKeySummaries, true},
		{"valid tags stream", StreamKeyTags, true},
		{"valid index stream", StreamKeyIndex, true},
		{"invalid stream", StreamKey("invalid"), false},
		{"empty stream", StreamKey(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.key.IsValid())
		})
	}
}

func TestConsumerGroup_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		group ConsumerGroup
		valid bool
	}{
		{"valid pre-processor group", ConsumerGroupPreProcessor, true},
		{"valid tag-generator group", ConsumerGroupTagGenerator, true},
		{"valid search-indexer group", ConsumerGroupSearchIndexer, true},
		{"invalid group", ConsumerGroup("invalid"), false},
		{"empty group", ConsumerGroup(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.group.IsValid())
		})
	}
}
