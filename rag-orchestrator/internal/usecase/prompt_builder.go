package usecase

import (
	"fmt"
	"rag-orchestrator/internal/domain"
	"strings"
)

// PromptContext transports the metadata needed when composing the generation prompt.
type PromptContext struct {
	ChunkID         string
	Title           string
	URL             string
	PublishedAt     string
	Score           float32
	DocumentVersion int
	ChunkText       string
}

// PromptInput contains the pieces that feed into the prompt builder.
type PromptInput struct {
	Query         string
	Locale        string
	PromptVersion string
	Contexts      []PromptContext
	Stage         string   // "citations" or "answer" (empty = combined)
	Citations     []string // For "answer" stage, pass previously extracted citations
}

// PromptBuilder builds the chat messages sent to the LLM.
type PromptBuilder interface {
	Build(input PromptInput) ([]domain.Message, error)
}

// XMLPromptBuilder creates structured prompts that separate context, instructions, query, and format.
type XMLPromptBuilder struct {
	additionalInstructions []string
}

// NewXMLPromptBuilder creates a prompt builder with optional extra instructions appended.
func NewXMLPromptBuilder(additionalInstructions ...string) PromptBuilder {
	return &XMLPromptBuilder{
		additionalInstructions: additionalInstructions,
	}
}

// Build renders the Messages for Chat API.
// Gemma 3 officially supports only user/model roles, so instructions are
// embedded in the user message instead of a separate system message.
func (b *XMLPromptBuilder) Build(input PromptInput) ([]domain.Message, error) {
	if input.PromptVersion == "" {
		return nil, fmt.Errorf("prompt version is required")
	}

	var sb strings.Builder

	// Instructions (merged into user message for Gemma 3 compatibility)
	sb.WriteString("## あなたの役割\n")
	sb.WriteString("あなたは優秀なリサーチアナリストです。提供されたコンテキスト情報を分析し、\n")
	sb.WriteString("ユーザーの質問に対して包括的で詳細な回答を生成してください。\n\n")

	sb.WriteString("## 回答の品質基準\n")
	sb.WriteString("- 回答は500文字以上で、具体的な事実・データ・事例を含むこと\n")
	sb.WriteString("- コンテキストの情報を最大限に活用し、複数のソースを統合すること\n")
	sb.WriteString("- ソース引用は[番号]形式（例: [1], [2]）で必ず付与すること\n\n")

	sb.WriteString("## 回答構造\n")
	sb.WriteString("1. **概要**: トピックの全体像を2-3文で説明\n")
	sb.WriteString("2. **詳細**: 具体的な事実・データ・事例を含む本文（最も重要なセクション）\n")
	sb.WriteString("   - 背景情報と現状\n")
	sb.WriteString("   - 具体的な内容・データ\n")
	sb.WriteString("   - 影響と意味合い\n")
	sb.WriteString("3. **まとめ**: 重要ポイントの整理と今後の展望\n\n")

	if len(b.additionalInstructions) > 0 {
		sb.WriteString("## 追加ルール\n")
		for _, inst := range b.additionalInstructions {
			sb.WriteString(fmt.Sprintf("- %s\n", inst))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## 出力形式\n")
	sb.WriteString("以下のJSON形式で出力してください。answer フィールドには Markdown を使用してください。\n")
	sb.WriteString("{\"answer\":\"## 概要\\n...\\n## 詳細\\n...\\n## まとめ\\n...\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"引用理由\"}],\"fallback\":false,\"reason\":\"\"}\n\n")
	sb.WriteString("コンテキストが不十分な場合は fallback=true とし、reason に理由を記述してください。\n\n")

	// Context
	sb.WriteString("### Context\n")
	for i, ctx := range input.Contexts {
		index := i + 1
		sb.WriteString(fmt.Sprintf("[%d] %s", index, ctx.Title))
		if ctx.PublishedAt != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", ctx.PublishedAt))
		}
		sb.WriteString("\n")
		sb.WriteString(ctx.ChunkText)
		sb.WriteString("\n\n")
	}

	sb.WriteString("### Query\n")
	sb.WriteString(input.Query)
	if input.Locale != "" {
		sb.WriteString(fmt.Sprintf("\n(Language: %s)", input.Locale))
	}

	return []domain.Message{
		{Role: "user", Content: sb.String()},
	}, nil
}
