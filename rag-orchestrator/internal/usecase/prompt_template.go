package usecase

import (
	"fmt"
	"rag-orchestrator/internal/domain"
	"strings"
)

// TemplateRegistry dispatches prompt building to intent-specific templates.
// Replaces the monolithic XMLPromptBuilder for alpha-v2 prompt version.
// Design principles (from research):
// - Position-aware: critical rules at beginning and end (instruction sandwich)
// - Intent-specific: each template contains only relevant instructions
// - No redundancy: "日本語で回答" appears at most twice (preamble + sandwich)
// - Slim: 60% smaller than the monolithic builder
type TemplateRegistry struct {
	templates map[IntentType]intentTemplate
	fallback  intentTemplate
}

// intentTemplate generates system and user messages for a specific intent.
type intentTemplate interface {
	buildSystem(input PromptInput) string
	buildUser(input PromptInput) string
	estimateSystemTokens() int
}

// NewTemplateRegistry creates a registry with all intent-specific templates.
func NewTemplateRegistry() *TemplateRegistry {
	general := &generalTemplate{}
	return &TemplateRegistry{
		templates: map[IntentType]intentTemplate{
			IntentGeneral:           general,
			IntentCausalExplanation: &causalTemplate{},
			IntentSynthesis:         &synthesisTemplate{},
			IntentComparison:        &comparisonTemplate{},
			IntentTemporal:          &temporalTemplate{},
			IntentFactCheck:         &factCheckTemplate{},
			IntentTopicDeepDive:     &deepDiveTemplate{},
			IntentArticleScoped:     &articleScopedTemplate{},
		},
		fallback: general,
	}
}

// Build renders the Messages for Chat API using intent-specific templates.
func (r *TemplateRegistry) Build(input PromptInput) ([]domain.Message, error) {
	if input.PromptVersion == "" {
		return nil, fmt.Errorf("prompt version is required")
	}

	tmpl := r.resolve(input.IntentType)

	if len(input.ConversationHistory) > 0 {
		return r.buildMultiTurn(tmpl, input)
	}
	return r.buildSingleTurn(tmpl, input)
}

// EstimateSystemTokens returns the estimated system prompt token count.
func (r *TemplateRegistry) EstimateSystemTokens(input PromptInput) int {
	tmpl := r.resolve(input.IntentType)
	return tmpl.estimateSystemTokens()
}

func (r *TemplateRegistry) resolve(intent IntentType) intentTemplate {
	if tmpl, ok := r.templates[intent]; ok {
		return tmpl
	}
	return r.fallback
}

func (r *TemplateRegistry) buildSingleTurn(tmpl intentTemplate, input PromptInput) ([]domain.Message, error) {
	system := tmpl.buildSystem(input)
	user := tmpl.buildUser(input)
	return []domain.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, nil
}

func (r *TemplateRegistry) buildMultiTurn(tmpl intentTemplate, input PromptInput) ([]domain.Message, error) {
	var msgs []domain.Message

	// System message with follow-up rules
	var sb strings.Builder
	sb.WriteString(tmpl.buildSystem(input))
	sb.WriteString("\n\n## フォローアップ指示\n")
	sb.WriteString("これは会話の続きです。前回の回答で述べた内容を繰り返さず、質問に直接回答すること。\n")
	msgs = append(msgs, domain.Message{Role: "system", Content: sb.String()})

	// Past turns
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
		msgs = append(msgs, domain.Message{Role: msg.Role, Content: content})
	}

	// User message
	msgs = append(msgs, domain.Message{Role: "user", Content: tmpl.buildUser(input)})
	return msgs, nil
}

// --- Shared helpers for templates ---

// preamble returns the standard role + constraint preamble (appears once).
func preamble() string {
	return "あなたはリサーチアナリストです。必ず日本語で回答してください。\n" +
		"提供されたコンテキスト情報のみに基づいて回答すること（外部知識を使わない）。\n" +
		"コンテキストに記載のない事実や数値を推測・捏造しないこと。\n" +
		"ソース引用[番号]を必ず付与すること。\n\n"
}

// outputFormatBrief returns a brief output format instruction.
// The full JSON schema is enforced by Ollama's generationFormat, not the prompt.
func outputFormatBrief() string {
	return "出力はJSON（answer, citations, fallback, reason）。answerにMarkdown使用。\n" +
		"コンテキストが不十分な場合はfallback=trueとしreasonに理由を記述。\n\n"
}

// sandwich returns the instruction sandwich (critical rules repeated at end).
func sandwich() string {
	return "【重要】日本語で回答。コンテキスト外の情報不可。引用[番号]必須。\n"
}

// lowConfidenceNote returns the low confidence disclaimer.
func lowConfidenceNote() string {
	return "\n## 情報の信頼性\n" +
		"ソースが限定的です。確認できた事実と推測を明確に区別し、不十分な箇所を明記すること。\n"
}

// buildUserMessage builds the user message with context chunks and query.
func buildUserMessage(input PromptInput) string {
	var sb strings.Builder

	if input.ArticleContext != nil {
		sb.WriteString(fmt.Sprintf("## 記事: %s\n\n", input.ArticleContext.Title))
	}

	if len(input.SupplementaryInfo) > 0 {
		sb.WriteString("### 補足情報\n")
		for _, info := range input.SupplementaryInfo {
			sb.WriteString(fmt.Sprintf("- %s\n", info))
		}
		sb.WriteString("\n")
	}

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
	return sb.String()
}

// estimateTokens estimates token count from character count.
// Japanese: ~2 chars per token, English: ~4 chars per token.
// We use ~3 as a conservative average for bilingual content.
func estimateTokens(text string) int {
	return len([]rune(text)) / 3
}
