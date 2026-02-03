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
func (b *XMLPromptBuilder) Build(input PromptInput) ([]domain.Message, error) {
	if input.PromptVersion == "" {
		return nil, fmt.Errorf("prompt version is required")
	}

	// 1. Build System Message (Instructions + Format)
	// Best practices: Role-based prompting, explicit structure with minimum character counts
	// Reference: Swallow official recommendation + long-form answer generation techniques
	// Phase 2: Enhanced prompting with detailed section guides for 8B models
	var sysSb strings.Builder
	sysSb.WriteString("あなたは誠実で優秀な日本人のリサーチアナリストです。質問に対して**非常に詳細で包括的な回答**を提供してください。\n\n")

	sysSb.WriteString("## 重要な注意事項\n")
	sysSb.WriteString("- 回答は**必ず2000文字以上**で作成してください\n")
	sysSb.WriteString("- 短い回答は不適切です。各セクションを十分に展開してください\n")
	sysSb.WriteString("- コンテキストに記載された情報を最大限活用し、詳しく説明してください\n\n")

	sysSb.WriteString("## 回答構造（全セクション必須）\n\n")

	sysSb.WriteString("### 1. 概要（200文字以上）\n")
	sysSb.WriteString("- トピックの全体像を説明\n")
	sysSb.WriteString("- 主要な論点を3つ以上挙げる\n")
	sysSb.WriteString("- 読者が何を学べるか示す\n\n")

	sysSb.WriteString("### 2. 詳細説明（1000文字以上・4段落以上）\n")
	sysSb.WriteString("- 第1段落: 基本的な事実と定義（誰が、何を、いつ、どこで）\n")
	sysSb.WriteString("- 第2段落: 背景と経緯（なぜこれが重要か、どのような流れで起きたか）\n")
	sysSb.WriteString("- 第3段落: 詳細な分析（具体的なデータ、数字、引用を含める）\n")
	sysSb.WriteString("- 第4段落: 関連する要素（他の事象との関連、波及効果）\n\n")

	sysSb.WriteString("### 3. 具体的な事例（500文字以上・2事例以上）\n")
	sysSb.WriteString("- 各事例について: 何が起きたか、誰が関与したか、結果はどうなったか\n")
	sysSb.WriteString("- コンテキストから具体的な名前、日付、数字を引用\n")
	sysSb.WriteString("- 各事例は最低3文以上で説明\n\n")

	sysSb.WriteString("### 4. 関連情報・背景（400文字以上）\n")
	sysSb.WriteString("- 歴史的な文脈\n")
	sysSb.WriteString("- 関連する人物や組織\n")
	sysSb.WriteString("- 社会的な影響や意味\n\n")

	sysSb.WriteString("### 5. まとめ（200文字以上）\n")
	sysSb.WriteString("- 重要なポイントを3つ以上列挙\n")
	sysSb.WriteString("- 今後の展望や示唆を含める\n\n")

	sysSb.WriteString("## ルール\n")
	sysSb.WriteString("- 各セクションは見出しだけでなく、十分な内容を含めること\n")
	sysSb.WriteString("- ソースの引用は[番号]形式で記載（例: [1], [2]）\n")
	sysSb.WriteString("- コンテキストから最低3つの異なるソースを引用すること\n")
	sysSb.WriteString("- コンテキストが不十分な場合は fallback=true を設定し、理由を説明\n")

	if len(b.additionalInstructions) > 0 {
		sysSb.WriteString("\n## Additional Rules\n")
		for _, inst := range b.additionalInstructions {
			sysSb.WriteString(fmt.Sprintf("- %s\n", inst))
		}
	}

	sysSb.WriteString("\n## Output Format (JSON only)\n")
	sysSb.WriteString("Respond with ONLY valid JSON in this exact schema:\n")
	sysSb.WriteString("```json\n")
	sysSb.WriteString("{\n")
	sysSb.WriteString("  \"answer\": \"## 概要\\n...\\n\\n## 詳細説明\\n...\\n\\n## 具体的な事例\\n...\\n\\n## 関連情報・背景\\n...\\n\\n## まとめ\\n...\",\n")
	sysSb.WriteString("  \"citations\": [\n")
	sysSb.WriteString("    {\"chunk_id\": \"1\", \"reason\": \"Used for overview statistics\"},\n")
	sysSb.WriteString("    {\"chunk_id\": \"3\", \"reason\": \"Source for example case\"}\n")
	sysSb.WriteString("  ],\n")
	sysSb.WriteString("  \"fallback\": false,\n")
	sysSb.WriteString("  \"reason\": \"Sufficient context available from sources 1, 3, 5\"\n")
	sysSb.WriteString("}\n")
	sysSb.WriteString("```\n")

	// 2. Build User Message (Context + Query)
	var userSb strings.Builder
	userSb.WriteString("### Context\n")
	for i, ctx := range input.Contexts {
		// Dense format: [Index] Title (Date): Content
		// Using 1-based index
		index := i + 1
		userSb.WriteString(fmt.Sprintf("[%d] %s", index, ctx.Title))
		if ctx.PublishedAt != "" {
			userSb.WriteString(fmt.Sprintf(" (%s)", ctx.PublishedAt))
		}
		userSb.WriteString("\n")
		userSb.WriteString(ctx.ChunkText)
		userSb.WriteString("\n\n")
	}

	userSb.WriteString("### Query\n")
	userSb.WriteString(input.Query)
	// Add Locale hint if present
	if input.Locale != "" {
		userSb.WriteString(fmt.Sprintf("\n(Language: %s)", input.Locale))
	}

	return []domain.Message{
		{Role: "system", Content: sysSb.String()},
		{Role: "user", Content: userSb.String()},
	}, nil
}
