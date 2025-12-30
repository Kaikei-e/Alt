package usecase

import (
	"fmt"
	"rag-orchestrator/internal/domain"
	"strings"
	"time"
)

// MorningLetterPromptInput defines the input for building morning letter prompts
type MorningLetterPromptInput struct {
	Query      string
	Contexts   []ContextItem
	Since      time.Time
	Until      time.Time
	TopicLimit int
	Locale     string
}

// MorningLetterPromptBuilder defines the interface for building morning letter prompts
type MorningLetterPromptBuilder interface {
	Build(input MorningLetterPromptInput) ([]domain.Message, error)
}

// xmlMorningLetterPromptBuilder implements MorningLetterPromptBuilder using XML-style formatting
type xmlMorningLetterPromptBuilder struct{}

// NewMorningLetterPromptBuilder creates a new morning letter prompt builder
func NewMorningLetterPromptBuilder() MorningLetterPromptBuilder {
	return &xmlMorningLetterPromptBuilder{}
}

// Build constructs the prompt messages for morning letter topic extraction
func (b *xmlMorningLetterPromptBuilder) Build(input MorningLetterPromptInput) ([]domain.Message, error) {
	if len(input.Contexts) == 0 {
		return nil, fmt.Errorf("no contexts provided")
	}

	topicLimit := input.TopicLimit
	if topicLimit <= 0 {
		topicLimit = 10
	}

	hoursWindow := int(input.Until.Sub(input.Since).Hours())

	var sysSb strings.Builder

	// System message with temporal context
	sysSb.WriteString("You are an expert news analyst specializing in identifying and summarizing important topics.\n")
	sysSb.WriteString("Reasoning: medium\n\n")

	sysSb.WriteString("### Task\n")
	sysSb.WriteString(fmt.Sprintf("Analyze news from the past %d hours and identify the most important topics.\n", hoursWindow))
	sysSb.WriteString(fmt.Sprintf("Time Window: %s to %s\n\n",
		input.Since.Format("2006-01-02 15:04 JST"),
		input.Until.Format("2006-01-02 15:04 JST")))

	sysSb.WriteString("### Instructions\n")
	sysSb.WriteString(fmt.Sprintf("1. Identify up to %d distinct important topics from the context.\n", topicLimit))
	sysSb.WriteString("2. For each topic, provide:\n")
	sysSb.WriteString("   - A concise topic name (2-5 words)\n")
	sysSb.WriteString("   - A one-line headline\n")
	sysSb.WriteString("   - A 2-3 sentence summary\n")
	sysSb.WriteString("   - Importance score (0.0-1.0)\n")
	sysSb.WriteString("   - Source article references [index]\n")
	sysSb.WriteString("3. Prioritize topics by: recency, breadth of coverage, potential impact.\n")
	sysSb.WriteString("4. If query is in Japanese, respond in Japanese.\n")
	sysSb.WriteString("5. Output MUST be valid JSON.\n\n")

	sysSb.WriteString("### Response Format\n")
	sysSb.WriteString("```json\n")
	sysSb.WriteString("{\n")
	sysSb.WriteString("  \"topics\": [\n")
	sysSb.WriteString("    {\n")
	sysSb.WriteString("      \"topic\": \"Topic Name\",\n")
	sysSb.WriteString("      \"headline\": \"One-line headline...\",\n")
	sysSb.WriteString("      \"summary\": \"2-3 sentence summary...\",\n")
	sysSb.WriteString("      \"importance\": 0.9,\n")
	sysSb.WriteString("      \"article_refs\": [1, 3, 5],\n")
	sysSb.WriteString("      \"keywords\": [\"keyword1\", \"keyword2\"]\n")
	sysSb.WriteString("    }\n")
	sysSb.WriteString("  ],\n")
	sysSb.WriteString("  \"meta\": {\n")
	sysSb.WriteString("    \"topics_found\": 3,\n")
	sysSb.WriteString("    \"coverage_assessment\": \"comprehensive|partial|limited\"\n")
	sysSb.WriteString("  }\n")
	sysSb.WriteString("}\n")
	sysSb.WriteString("```\n")

	// User message with context
	var userSb strings.Builder
	userSb.WriteString("### Context (Recent News)\n")
	for i, ctx := range input.Contexts {
		index := i + 1
		userSb.WriteString(fmt.Sprintf("[%d] %s (%s)\n", index, ctx.Title, ctx.PublishedAt))
		userSb.WriteString(ctx.ChunkText)
		userSb.WriteString("\n\n")
	}

	userSb.WriteString("### User Query\n")
	userSb.WriteString(input.Query)
	if input.Locale != "" {
		userSb.WriteString(fmt.Sprintf("\n(Preferred Language: %s)", input.Locale))
	}

	return []domain.Message{
		{Role: "system", Content: sysSb.String()},
		{Role: "user", Content: userSb.String()},
	}, nil
}
