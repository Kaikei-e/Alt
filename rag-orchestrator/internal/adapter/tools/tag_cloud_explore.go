package tools

import (
	"context"
	"fmt"
	"strings"

	"rag-orchestrator/internal/domain"
)

// TagCloudExploreTool explores the tag space to find related topic areas.
// Uses alt-backend FetchTagCloud RPC to discover tags relevant to a topic.
type TagCloudExploreTool struct {
	client domain.TagCloudClient
}

// NewTagCloudExploreTool creates a new tag cloud explore tool.
func NewTagCloudExploreTool(client domain.TagCloudClient) *TagCloudExploreTool {
	return &TagCloudExploreTool{client: client}
}

func (t *TagCloudExploreTool) Name() string { return "tag_cloud_explore" }
func (t *TagCloudExploreTool) Description() string {
	return "Explore tag space to find related topic areas. Params: topic"
}

func (t *TagCloudExploreTool) Execute(ctx context.Context, params map[string]string) (*domain.ToolResult, error) {
	topic := strings.TrimSpace(params["topic"])
	if topic == "" {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Error:    "topic is required",
		}, nil
	}

	tags, err := t.client.FetchTagCloud(ctx, 300)
	if err != nil {
		return nil, fmt.Errorf("tag cloud fetch failed: %w", err)
	}

	// Split topic into keywords for matching
	keywords := strings.Fields(strings.ToLower(topic))

	var matched []domain.TagCloudEntry
	for _, tag := range tags {
		tagLower := strings.ToLower(tag.TagName)
		for _, kw := range keywords {
			if strings.Contains(tagLower, kw) || strings.Contains(kw, tagLower) {
				matched = append(matched, tag)
				break
			}
		}
	}

	if len(matched) == 0 {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Data:     "no matching tags found for topic: " + topic,
			Success:  true,
		}, nil
	}

	// Limit to top 10 most relevant tags
	if len(matched) > 10 {
		matched = matched[:10]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Related tags for '%s':\n", topic))
	for _, tag := range matched {
		sb.WriteString(fmt.Sprintf("- %s (%d articles)\n", tag.TagName, tag.ArticleCount))
	}

	return &domain.ToolResult{
		ToolName: t.Name(),
		Data:     sb.String(),
		Success:  true,
	}, nil
}
