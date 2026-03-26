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
	Query               string
	Locale              string
	PromptVersion       string
	Contexts            []PromptContext
	Stage               string           // "citations" or "answer" (empty = combined)
	Citations           []string         // For "answer" stage, pass previously extracted citations
	ConversationHistory []domain.Message // Recent chat turns for multi-turn context
	ArticleContext      *ArticleContext  // Set when query targets a specific article
	IntentType          IntentType       // Classified query intent (Phase 2)
	SupplementaryInfo   []string         // Tool results injected as supplementary context (Phase 3)
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
// Single-turn: one user message with full instructions + context + query.
// Multi-turn: past turns as actual user/assistant messages + follow-up user message.
func (b *XMLPromptBuilder) Build(input PromptInput) ([]domain.Message, error) {
	if input.PromptVersion == "" {
		return nil, fmt.Errorf("prompt version is required")
	}

	if len(input.ConversationHistory) > 0 {
		return b.buildMultiTurn(input)
	}
	return b.buildSingleTurn(input)
}

// buildSingleTurn creates a single user message with full instructions, context, and query.
func (b *XMLPromptBuilder) buildSingleTurn(input PromptInput) ([]domain.Message, error) {
	var sb strings.Builder

	b.writeFullInstructions(&sb, input)
	b.writeOutputFormat(&sb)
	b.writeArticleContext(&sb, input)
	b.writeSupplementaryInfo(&sb, input)
	b.writeContextChunks(&sb, input)
	b.writeQuery(&sb, input)

	return []domain.Message{
		{Role: "user", Content: sb.String()},
	}, nil
}

// buildMultiTurn creates actual chat turns for past conversation, plus a follow-up
// user message with context and query. This leverages the LLM's native multi-turn
// understanding (Gemma: <start_of_turn>user / <start_of_turn>model) instead of
// embedding history as text in a single message.
func (b *XMLPromptBuilder) buildMultiTurn(input PromptInput) ([]domain.Message, error) {
	var messages []domain.Message

	// Past turns as actual user/assistant chat messages.
	// Ollama maps "assistant" to model-specific tokens automatically.
	maxMsgs := 6
	start := 0
	if len(input.ConversationHistory) > maxMsgs {
		start = len(input.ConversationHistory) - maxMsgs
	}
	for _, msg := range input.ConversationHistory[start:] {
		content := msg.Content
		if len(content) > 3000 {
			content = content[:3000] + "..."
		}
		messages = append(messages, domain.Message{
			Role:    msg.Role,
			Content: content,
		})
	}

	// Current turn: follow-up instructions + context + query
	var sb strings.Builder

	sb.WriteString("## フォローアップ指示\n")
	sb.WriteString("これは会話の続きです。以下のルールに必ず従ってください:\n")
	sb.WriteString("- **前回の回答で既に述べた内容を一切繰り返さないこと**\n")
	sb.WriteString("- 概要の再説明は不要。質問に直接回答すること\n")
	sb.WriteString("- 前回触れていない新しい事実・データ・視点のみを提供すること\n")
	sb.WriteString("- 必ず日本語で回答すること\n\n")

	b.writeFollowUpOutputFormat(&sb)
	b.writeArticleContext(&sb, input)
	b.writeSupplementaryInfo(&sb, input)
	b.writeContextChunks(&sb, input)
	b.writeQuery(&sb, input)

	messages = append(messages, domain.Message{
		Role:    "user",
		Content: sb.String(),
	})

	return messages, nil
}

// --- Shared prompt section writers ---

func (b *XMLPromptBuilder) writeFullInstructions(sb *strings.Builder, input PromptInput) {
	sb.WriteString("## あなたの役割\n")
	sb.WriteString("あなたは優秀なリサーチアナリストです。提供されたコンテキスト情報を分析し、\n")
	sb.WriteString("ユーザーの質問に対して包括的で詳細な回答を生成してください。\n\n")

	sb.WriteString("## 回答の品質基準\n")
	sb.WriteString("- **必ず日本語で回答すること**。ソース記事が英語であっても、回答は日本語で記述すること\n")
	sb.WriteString("- 結論を最初に述べ、その後で根拠と詳細を説明すること\n")
	sb.WriteString("- 回答は800文字以上で、具体的な事実・データ・事例を含むこと\n")
	sb.WriteString("- コンテキストの情報を最大限に活用し、複数のソースを統合すること\n")
	sb.WriteString("- ソース引用は[番号]形式（例: [1], [2]）で必ず付与すること\n")
	sb.WriteString("- 提供されたコンテキスト情報のみに基づいて回答すること（外部知識を使わない）\n")
	sb.WriteString("- コンテキストに記載のない事実や数値を推測・捏造しないこと\n")
	sb.WriteString("- 情報が不十分な場合は、不足している点を明示すること\n\n")

	sb.WriteString("## 回答構造\n")
	sb.WriteString("1. **概要**: 結論と全体像を2-3文で説明（最重要ポイントを冒頭に）\n")
	sb.WriteString("2. **詳細**: 具体的な事実・データ・事例を含む本文（最も重要なセクション）\n")
	sb.WriteString("   - 背景情報と現状（コンテキストから引用、[番号]で出典明記）\n")
	sb.WriteString("   - 具体的な内容・データ（数値・日付・固有名詞を正確に引用）\n")
	sb.WriteString("   - 影響と意味合い（複数ソースの情報を統合して分析）\n")
	sb.WriteString("3. **まとめ**: 重要ポイントの整理と今後の展望\n\n")

	if len(b.additionalInstructions) > 0 {
		sb.WriteString("## 追加ルール\n")
		for _, inst := range b.additionalInstructions {
			sb.WriteString(fmt.Sprintf("- %s\n", inst))
		}
		sb.WriteString("\n")
	}

	// Intent-specific instructions (Phase 2: Agentic RAG)
	switch input.IntentType {
	case IntentComparison:
		sb.WriteString("## クエリ意図: 比較\n")
		sb.WriteString("- 両者を公平に比較し、共通点・相違点を構造化してください\n")
		sb.WriteString("- 一方に偏らず、各項目の長所・短所を併記してください\n\n")
	case IntentTemporal:
		sb.WriteString("## クエリ意図: 時系列\n")
		sb.WriteString("- 最新の情報を優先して回答してください\n")
		sb.WriteString("- 日付・時期を明記し、時系列順に整理してください\n\n")
	case IntentTopicDeepDive:
		sb.WriteString("## クエリ意図: 深掘り\n")
		sb.WriteString("- 背景・詳細・影響を包括的に解説してください\n")
		sb.WriteString("- 基本概念から応用まで段階的に説明してください\n\n")
	case IntentFactCheck:
		sb.WriteString("## クエリ意図: ファクトチェック\n")
		sb.WriteString("- 出典を明示し、根拠と判定を構造化して回答してください\n")
		sb.WriteString("- 「主張」「根拠」「判定」の3段構成で回答してください\n\n")
	}
}

func (b *XMLPromptBuilder) writeFollowUpOutputFormat(sb *strings.Builder) {
	sb.WriteString("## 出力形式\n")
	sb.WriteString("以下のJSON形式で出力してください。answer フィールドには Markdown を使用してください。\n")
	sb.WriteString("**概要セクションは不要です。質問への回答を直接書いてください。**\n")
	sb.WriteString("{\"answer\":\"質問への直接回答をここに書く\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"引用理由\"}],\"fallback\":false,\"reason\":\"\"}\n\n")
	sb.WriteString("コンテキストが不十分な場合は fallback=true とし、reason に理由を記述してください。\n\n")
}

func (b *XMLPromptBuilder) writeOutputFormat(sb *strings.Builder) {
	sb.WriteString("## 出力形式\n")
	sb.WriteString("以下のJSON形式で出力してください。answer フィールドには Markdown を使用してください。\n")
	sb.WriteString("{\"answer\":\"## 概要\\n...\\n## 詳細\\n...\\n## まとめ\\n...\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"引用理由\"}],\"fallback\":false,\"reason\":\"\"}\n\n")
	sb.WriteString("コンテキストが不十分な場合は fallback=true とし、reason に理由を記述してください。\n\n")
}

func (b *XMLPromptBuilder) writeArticleContext(sb *strings.Builder, input PromptInput) {
	if input.ArticleContext != nil {
		if input.ArticleContext.Truncated {
			sb.WriteString(fmt.Sprintf("## 記事コンテキスト\n以下は記事「%s」の主要な部分です。この記事に基づいて質問に回答してください。\n\n", input.ArticleContext.Title))
		} else {
			sb.WriteString(fmt.Sprintf("## 記事コンテキスト\n以下は記事「%s」の全内容です。この記事に基づいて質問に回答してください。\n\n", input.ArticleContext.Title))
		}
	}
}

func (b *XMLPromptBuilder) writeSupplementaryInfo(sb *strings.Builder, input PromptInput) {
	if len(input.SupplementaryInfo) > 0 {
		sb.WriteString("### 補足情報（ツール結果）\n")
		for _, info := range input.SupplementaryInfo {
			sb.WriteString(fmt.Sprintf("- %s\n", info))
		}
		sb.WriteString("\n")
	}
}

func (b *XMLPromptBuilder) writeContextChunks(sb *strings.Builder, input PromptInput) {
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
}

func (b *XMLPromptBuilder) writeQuery(sb *strings.Builder, input PromptInput) {
	sb.WriteString("### Query\n")
	sb.WriteString(input.Query)
	if input.Locale != "" {
		sb.WriteString(fmt.Sprintf("\n(Language: %s)", input.Locale))
	}
}
