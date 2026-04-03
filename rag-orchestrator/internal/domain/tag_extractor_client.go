package domain

import "context"

// ExtractedTag represents a tag extracted from text by the tag-generator.
type ExtractedTag struct {
	Name       string
	Confidence float32
}

// TagExtractorClient extracts semantic tags from text via mq-hub → tag-generator.
type TagExtractorClient interface {
	ExtractTags(ctx context.Context, text string) ([]ExtractedTag, error)
}
