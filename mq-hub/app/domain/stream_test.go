package domain

import (
	"errors"
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

func TestPartialPublishError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *PartialPublishError
		wantMsg string
	}{
		{
			name: "single failure reports its index and total",
			err: &PartialPublishError{
				TotalEvents: 3,
				Failures: []PublishFailure{
					{Index: 1, Err: errors.New("connection reset")},
				},
			},
			wantMsg: "partial publish failure (1/3 events failed): index 1: connection reset",
		},
		{
			name: "multiple failures are joined and keep their own index",
			err: &PartialPublishError{
				TotalEvents: 4,
				Failures: []PublishFailure{
					{Index: 0, Err: errors.New("timeout")},
					{Index: 2, Err: errors.New("connection reset")},
				},
			},
			wantMsg: "partial publish failure (2/4 events failed): index 0: timeout; index 2: connection reset",
		},
		{
			name: "no failures still reports zero-of-total",
			err: &PartialPublishError{
				TotalEvents: 2,
				Failures:    nil,
			},
			wantMsg: "partial publish failure (0/2 events failed): ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMsg, tt.err.Error())
		})
	}
}

func TestPartialPublishError_ErrorsAs(t *testing.T) {
	// Callers (handler.mapPublishErr, PublishUsecase.PublishBatch) classify
	// this error via errors.As instead of a type assertion; pin that it
	// actually satisfies that contract when wrapped.
	original := &PartialPublishError{
		TotalEvents: 1,
		Failures:    []PublishFailure{{Index: 0, Err: errors.New("boom")}},
	}
	wrapped := errors.New("publish batch to alt:events:articles: " + original.Error())

	var target *PartialPublishError
	assert.False(t, errors.As(wrapped, &target), "a plain wrapped string must not be mistaken for the typed error")

	var direct *PartialPublishError
	assert.True(t, errors.As(error(original), &direct))
	assert.Same(t, original, direct)
}
