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

// Build constructs the prompt messages for morning letter topic extraction.
// Prompt is in Japanese for Gemma 3 token efficiency and output quality.
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

	sysSb.WriteString("あなたは優秀なニュースアナリストです。最近のニュース記事を分析し、重要なトピックを特定・要約してください。\n\n")

	sysSb.WriteString("### タスク\n")
	sysSb.WriteString(fmt.Sprintf("過去%d時間のニュースを分析し、最も重要なトピックを特定してください。\n", hoursWindow))
	sysSb.WriteString(fmt.Sprintf("分析期間: %s 〜 %s\n\n",
		input.Since.Format("2006-01-02 15:04 JST"),
		input.Until.Format("2006-01-02 15:04 JST")))

	sysSb.WriteString("### 指示\n")
	sysSb.WriteString(fmt.Sprintf("1. コンテキストから最大%d個の重要トピックを特定する\n", topicLimit))
	sysSb.WriteString("2. 各トピックについて以下を提供する:\n")
	sysSb.WriteString("   - トピック名（2-5語）\n")
	sysSb.WriteString("   - 一行の見出し\n")
	sysSb.WriteString("   - 詳細な要約（8-12文、250文字以上）:\n")
	sysSb.WriteString("     * 主要なニュース事実と進展\n")
	sysSb.WriteString("     * 読者向けの背景情報\n")
	sysSb.WriteString("     * なぜ重要か、潜在的な影響\n")
	sysSb.WriteString("     * ソースからの重要なデータポイント\n")
	sysSb.WriteString("   - 重要度スコア（0.0-1.0）\n")
	sysSb.WriteString("   - ソース記事の参照番号\n")
	sysSb.WriteString("3. 優先順位: 新しさ、カバー範囲の広さ、潜在的影響\n")
	sysSb.WriteString("4. 出力は必ず有効なJSONで返すこと\n\n")

	sysSb.WriteString("### 出力形式（JSONのみ）\n")
	sysSb.WriteString("```json\n")
	sysSb.WriteString("{\n")
	sysSb.WriteString("  \"topics\": [\n")
	sysSb.WriteString("    {\n")
	sysSb.WriteString("      \"topic\": \"トピック名\",\n")
	sysSb.WriteString("      \"headline\": \"一行見出し...\",\n")
	sysSb.WriteString("      \"summary\": \"8-12文の詳細要約...\",\n")
	sysSb.WriteString("      \"importance\": 0.9,\n")
	sysSb.WriteString("      \"article_refs\": [1, 3, 5],\n")
	sysSb.WriteString("      \"keywords\": [\"キーワード1\", \"キーワード2\"]\n")
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
	userSb.WriteString("### コンテキスト（最近のニュース）\n")
	for i, ctx := range input.Contexts {
		index := i + 1
		userSb.WriteString(fmt.Sprintf("[%d] %s (%s)\n", index, ctx.Title, ctx.PublishedAt))
		userSb.WriteString(ctx.ChunkText)
		userSb.WriteString("\n\n")
	}

	userSb.WriteString("### クエリ\n")
	userSb.WriteString(input.Query)
	if input.Locale != "" {
		userSb.WriteString(fmt.Sprintf("\n(言語: %s)", input.Locale))
	}

	return []domain.Message{
		{Role: "system", Content: sysSb.String()},
		{Role: "user", Content: userSb.String()},
	}, nil
}
