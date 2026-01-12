package domain

// StreamKey represents a Redis Stream key.
type StreamKey string

// Stream keys for the Alt platform.
const (
	// StreamKeyArticles is the stream for article lifecycle events.
	StreamKeyArticles StreamKey = "alt:events:articles"
	// StreamKeySummaries is the stream for summarization events.
	StreamKeySummaries StreamKey = "alt:events:summaries"
	// StreamKeyTags is the stream for tag generation events.
	StreamKeyTags StreamKey = "alt:events:tags"
	// StreamKeyIndex is the stream for index commands.
	StreamKeyIndex StreamKey = "alt:events:index"
)

// validStreamKeys contains all valid stream keys.
var validStreamKeys = map[StreamKey]bool{
	StreamKeyArticles:  true,
	StreamKeySummaries: true,
	StreamKeyTags:      true,
	StreamKeyIndex:     true,
}

// IsValid returns true if the stream key is a known valid key.
func (s StreamKey) IsValid() bool {
	return validStreamKeys[s]
}

// String returns the string representation of the stream key.
func (s StreamKey) String() string {
	return string(s)
}

// ConsumerGroup represents a Redis consumer group name.
type ConsumerGroup string

// Consumer groups for the Alt platform.
const (
	// ConsumerGroupPreProcessor is the group for pre-processor service.
	ConsumerGroupPreProcessor ConsumerGroup = "pre-processor-group"
	// ConsumerGroupTagGenerator is the group for tag-generator service.
	ConsumerGroupTagGenerator ConsumerGroup = "tag-generator-group"
	// ConsumerGroupSearchIndexer is the group for search-indexer service.
	ConsumerGroupSearchIndexer ConsumerGroup = "search-indexer-group"
)

// validConsumerGroups contains all valid consumer groups.
var validConsumerGroups = map[ConsumerGroup]bool{
	ConsumerGroupPreProcessor:  true,
	ConsumerGroupTagGenerator:  true,
	ConsumerGroupSearchIndexer: true,
}

// IsValid returns true if the consumer group is a known valid group.
func (c ConsumerGroup) IsValid() bool {
	return validConsumerGroups[c]
}

// String returns the string representation of the consumer group.
func (c ConsumerGroup) String() string {
	return string(c)
}

// StreamInfo contains metadata about a Redis Stream.
type StreamInfo struct {
	// Length is the number of entries in the stream.
	Length int64
	// RadixTreeKeys is the number of radix tree keys.
	RadixTreeKeys int64
	// RadixTreeNodes is the number of radix tree nodes.
	RadixTreeNodes int64
	// FirstEntryID is the ID of the first entry.
	FirstEntryID string
	// LastEntryID is the ID of the last entry.
	LastEntryID string
	// Groups contains information about consumer groups.
	Groups []ConsumerGroupInfo
}

// ConsumerGroupInfo contains metadata about a consumer group.
type ConsumerGroupInfo struct {
	// Name is the consumer group name.
	Name string
	// Consumers is the number of consumers in the group.
	Consumers int64
	// Pending is the number of pending messages.
	Pending int64
	// LastDeliveredID is the ID of the last delivered message.
	LastDeliveredID string
}
