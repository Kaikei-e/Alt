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
	Stage               string                // "citations" or "answer" (empty = combined)
	Citations           []string              // For "answer" stage, pass previously extracted citations
	ConversationHistory []domain.Message      // Recent chat turns for multi-turn context
	ArticleContext      *ArticleContext       // Set when query targets a specific article
	IntentType          IntentType            // Classified query intent (Phase 2)
	SubIntentType       SubIntentType         // Analytical sub-intent for article-scoped queries
	SupplementaryInfo   []string              // Tool results injected as supplementary context (Phase 3)
	PlannerOutput       *domain.PlannerOutput // Planner-driven prompt shaping (nil = legacy mode)
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
	switch {
	case input.SubIntentType == SubIntentRelatedArticles:
		b.writeRelatedArticlesOutputFormat(&sb)
	case input.SubIntentType != SubIntentNone:
		b.writeAnalyticalOutputFormat(&sb)
	default:
		b.writeOutputFormat(&sb)
	}
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
	sb.WriteString("- 必ず日本語で回答すること\n")

	// Sub-intent specific follow-up guidance
	switch input.SubIntentType {
	case SubIntentCritique:
		sb.WriteString("- **記事の要約を繰り返さず、主張の弱点・反証可能性・欠落前提に集中すること**\n")
		sb.WriteString("- 反対の立場からの視点や、記事が見落としている点を指摘すること\n")
	case SubIntentOpinion:
		sb.WriteString("- **要約ではなく、分析的な評価と意見を述べること**\n")
	case SubIntentImplication:
		sb.WriteString("- **要約ではなく、影響・意味合い・今後の展望を分析すること**\n")
	case SubIntentDetail:
		sb.WriteString("- **概要を繰り返さず、技術的な詳細・メカニズム・ステップに集中すること**\n")
		sb.WriteString("- 具体的なデータや数値があれば正確に引用すること\n")
	case SubIntentRelatedArticles:
		sb.WriteString("- **関連記事のランク付きリストを返すこと。散文形式にしないこと**\n")
	case SubIntentEvidence:
		sb.WriteString("- **主張の根拠となる具体的なパッセージを引用付きで返すこと**\n")
	case SubIntentSummaryRefresh:
		sb.WriteString("- **簡潔な要約を提供すること。前回の回答との重複は許容**\n")
	}
	sb.WriteString("\n")

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
	if input.SubIntentType != SubIntentNone {
		sb.WriteString("- 回答は具体的な事実・データ・事例を含むこと\n")
	} else {
		sb.WriteString("- 回答は800文字以上で、具体的な事実・データ・事例を含むこと\n")
	}
	sb.WriteString("- コンテキストの情報を最大限に活用し、複数のソースを統合すること\n")
	sb.WriteString("- ソース引用は[番号]形式（例: [1], [2]）で必ず付与すること\n")
	sb.WriteString("- 提供されたコンテキスト情報のみに基づいて回答すること（外部知識を使わない）\n")
	sb.WriteString("- コンテキストに記載のない事実や数値を推測・捏造しないこと\n")
	sb.WriteString("- 情報が不十分な場合は、不足している点を明示すること\n\n")

	// Generic summary structure only for queries without a specific SubIntent.
	// SubIntents have their own task-specific guidance below, and mixing in
	// 概要/詳細/まとめ sends the model conflicting signals about answer shape.
	if input.SubIntentType == SubIntentNone && input.IntentType != IntentCausalExplanation && input.IntentType != IntentFactCheck && input.IntentType != IntentSynthesis {
		sb.WriteString("## 回答構造\n")
		sb.WriteString("1. **概要**: 結論と全体像を2-3文で説明（最重要ポイントを冒頭に）\n")
		sb.WriteString("2. **詳細**: 具体的な事実・データ・事例を含む本文（最も重要なセクション）\n")
		sb.WriteString("   - 背景情報と現状（コンテキストから引用、[番号]で出典明記）\n")
		sb.WriteString("   - 具体的な内容・データ（数値・日付・固有名詞を正確に引用）\n")
		sb.WriteString("   - 影響と意味合い（複数ソースの情報を統合して分析）\n")
		sb.WriteString("3. **まとめ**: 重要ポイントの整理と今後の展望\n\n")
	}

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
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("- 両者を公平に比較し、共通点・相違点を構造化してください\n")
		sb.WriteString("- 一方に偏らず、各項目の長所・短所を併記してください\n\n")
	case IntentTemporal:
		sb.WriteString("## クエリ意図: 時系列\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("- 最新の情報を優先して回答してください\n")
		sb.WriteString("- 日付・時期を明記し、時系列順に整理してください\n\n")
	case IntentTopicDeepDive:
		sb.WriteString("## クエリ意図: 深掘り\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("- 背景・詳細・影響を包括的に解説してください\n")
		sb.WriteString("- 基本概念から応用まで段階的に説明してください\n\n")
	case IntentFactCheck:
		sb.WriteString("## クエリ意図: ファクトチェック\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("- 出典を明示し、根拠と判定を構造化して回答してください\n")
		sb.WriteString("- 「主張」「根拠」「判定」の3段構成で回答してください\n\n")
	case IntentCausalExplanation:
		sb.WriteString("## クエリ意図: 因果分析\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("- 回答は最低3つの構成要素で構造化すること:\n")
		sb.WriteString("  1. **直接的要因**: 直近のトリガーとなった出来事\n")
		sb.WriteString("  2. **構造的背景**: 長期的な要因・制度的背景\n")
		sb.WriteString("  3. **不確実性**: 根拠が不十分な点、見解が分かれる点\n")
		sb.WriteString("- 単一の原因に帰結させず、複数の要因を分離して記述すること\n")
		sb.WriteString("- ソースが収束しない場合は「見解が分かれている」と明記すること\n")
		sb.WriteString("- 各要因にソース引用[番号]を必ず付けること\n\n")
	case IntentSynthesis:
		sb.WriteString("## クエリ意図: 概念的合成\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("- ユーザーは広範なテーマについての包括的な理解を求めています\n")
		sb.WriteString("- 回答は以下の構造で作成すること:\n")
		sb.WriteString("  1. **導入**: テーマの概要と主要な側面を2-3文で提示\n")
		sb.WriteString("  2. **多面的分析**: 3つ以上の異なる側面・視点から論じること\n")
		sb.WriteString("     - 各側面にサブ見出し（**太字**）を付与\n")
		sb.WriteString("     - 各側面にソース引用[番号]を必ず付与\n")
		sb.WriteString("  3. **相互関係**: 側面間のつながりや影響関係を記述\n")
		sb.WriteString("  4. **現状と展望**: 最新の動向と今後の方向性\n")
		sb.WriteString("- 1つの側面だけに偏らず、バランスよく複数の視点を提供すること\n")
		sb.WriteString("- コンテキストに情報が不十分な側面は「この側面については情報が限定的です」と明記\n")
		sb.WriteString("- 回答は1200文字以上で、具体的な事実・データ・事例を含むこと\n\n")
	}

	// Sub-intent-specific instructions for article-scoped queries
	switch input.SubIntentType {
	case SubIntentCritique:
		sb.WriteString("## クエリ意図: 批判的分析\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("ユーザーは記事の内容に対する反論・批判・弱点を知りたいと思っています。\n")
		sb.WriteString("- 記事の主張を簡潔に述べた上で、それに対する反論・批判を提示すること\n")
		sb.WriteString("- 考えられる弱点・限界・問題点を具体的に列挙すること\n")
		sb.WriteString("- 反対の立場からの視点や、記事が見落としている点を指摘すること\n")
		sb.WriteString("- **記事の内容を要約するのではなく、批判的に分析すること**\n\n")
	case SubIntentOpinion:
		sb.WriteString("## クエリ意図: 評価・意見\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("ユーザーは記事の内容に対する評価や分析的な意見を求めています。\n")
		sb.WriteString("- コンテキストの情報に基づいて、分析的な評価を提示すること\n")
		sb.WriteString("- 長所と短所の両面から評価すること\n")
		sb.WriteString("- **記事の内容を要約するのではなく、分析・評価を行うこと**\n\n")
	case SubIntentImplication:
		sb.WriteString("## クエリ意図: 影響・意味合い\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("ユーザーは記事の内容がもたらす影響や意味合いを知りたいと思っています。\n")
		sb.WriteString("- 記事の内容が何を意味するのか、その影響を分析すること\n")
		sb.WriteString("- 短期的・長期的な影響を区別して説明すること\n")
		sb.WriteString("- **記事の内容を要約するのではなく、その影響と意味合いを分析すること**\n\n")
	case SubIntentDetail:
		sb.WriteString("## クエリ意図: 技術的詳細\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("ユーザーは記事の技術的な詳細、メカニズム、ステップを知りたいと思っています。\n")
		sb.WriteString("- 質問に直接回答すること。概要の再説明は不要\n")
		sb.WriteString("- メカニズム・手順・技術的根拠に焦点を当てること\n")
		sb.WriteString("- 具体的なデータ・数値を正確に引用すること\n")
		sb.WriteString("- **記事の要約ではなく、技術的な詳細に集中すること**\n\n")
	case SubIntentRelatedArticles:
		sb.WriteString("## クエリ意図: 関連記事\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("ユーザーはこの記事に関連する他の記事を知りたいと思っています。\n")
		sb.WriteString("- 関連記事のランク付きリストを返すこと（散文形式ではなくリスト形式）\n")
		sb.WriteString("- 各記事に関連する理由を1文で説明すること\n")
		sb.WriteString("- **長文の散文ではなく、簡潔な構造化リストで回答すること**\n\n")
	case SubIntentEvidence:
		sb.WriteString("## クエリ意図: 根拠・エビデンス\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("ユーザーは記事の主張を裏付ける具体的な根拠を求めています。\n")
		sb.WriteString("- 引用付きで具体的なパッセージを返すこと\n")
		sb.WriteString("- 各引用に出典番号[番号]を付与すること\n")
		sb.WriteString("- 根拠の強さ（直接的か間接的か）を明示すること\n")
		sb.WriteString("- **記事の要約ではなく、根拠となるパッセージに集中すること**\n\n")
	case SubIntentSummaryRefresh:
		sb.WriteString("## クエリ意図: 要約リフレッシュ\n")
		sb.WriteString("- **必ず日本語で回答すること**\n")
		sb.WriteString("ユーザーは記事の簡潔な要約を求めています。\n")
		sb.WriteString("- 重要なポイントを簡潔にまとめること\n")
		sb.WriteString("- 前回の回答と重複しても構わない（ユーザーが要約を求めている）\n")
		sb.WriteString("- **簡潔さを優先し、5-7ポイントに絞ること**\n\n")
	}
}

func (b *XMLPromptBuilder) writeAnalyticalOutputFormat(sb *strings.Builder) {
	sb.WriteString("## 出力形式\n")
	sb.WriteString("以下のJSON形式で出力してください。answer フィールドには Markdown を使用してください。\n")
	sb.WriteString("**重要: 記事の要約ではなく、質問に対する分析的な回答を書いてください。**\n")
	sb.WriteString("{\"answer\":\"質問に対する分析的な回答をここに書く\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"引用理由\"}],\"fallback\":false,\"reason\":\"\"}\n\n")
	sb.WriteString("コンテキストが不十分な場合は fallback=true とし、reason に理由を記述してください。\n\n")
}

func (b *XMLPromptBuilder) writeFollowUpOutputFormat(sb *strings.Builder) {
	sb.WriteString("## 出力形式\n")
	sb.WriteString("以下のJSON形式で出力してください。answer フィールドには Markdown を使用してください。\n")
	sb.WriteString("**概要セクションは不要です。質問への回答を直接書いてください。**\n")
	sb.WriteString("{\"answer\":\"質問への直接回答をここに書く\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"引用理由\"}],\"fallback\":false,\"reason\":\"\"}\n\n")
	sb.WriteString("コンテキストが不十分な場合は fallback=true とし、reason に理由を記述してください。\n\n")
}

func (b *XMLPromptBuilder) writeRelatedArticlesOutputFormat(sb *strings.Builder) {
	sb.WriteString("## 出力形式\n")
	sb.WriteString("以下のJSON形式で出力してください。answer フィールドにはMarkdownリストを使用してください。\n")
	sb.WriteString("**重要: 散文ではなく、関連記事の簡潔なランク付きリストで回答してください。**\n")
	sb.WriteString("{\"answer\":\"関連記事のリストをここに書く\",")
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
