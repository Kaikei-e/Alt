package usecase

import (
	"strings"
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- PromptTemplate Interface ---

func TestTemplateRegistry_DispatchesByIntent(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "イランの石油危機はなぜ起きた？",
		PromptVersion: "alpha-v2",
		IntentType:    IntentCausalExplanation,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Iran Oil Crisis", ChunkText: "制裁が原因で石油供給が減少した"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)
	require.Len(t, msgs, 2, "should produce system + user messages")
	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, "user", msgs[1].Role)
}

func TestTemplateRegistry_CausalTemplate_HasRequiredStructure(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "なぜ石油危機が起きたのか？",
		PromptVersion: "alpha-v2",
		IntentType:    IntentCausalExplanation,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)

	system := msgs[0].Content
	// Must contain causal-specific structure guidance
	assert.Contains(t, system, "直接的要因")
	assert.Contains(t, system, "構造的背景")
	assert.Contains(t, system, "不確実性")
	// Must NOT contain generic 概要/詳細/まとめ structure
	assert.NotContains(t, system, "## 回答構造")
}

func TestTemplateRegistry_GeneralTemplate_HasDefaultStructure(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "最近のAI規制の動向は？",
		PromptVersion: "alpha-v2",
		IntentType:    IntentGeneral,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "AI Regulation", ChunkText: "EU AI Act..."},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)

	system := msgs[0].Content
	assert.Contains(t, system, "概要")
	assert.Contains(t, system, "詳細")
}

func TestTemplateRegistry_SynthesisTemplate_HasMultiFacetStructure(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "AIと社会の関係について包括的に解説して",
		PromptVersion: "alpha-v2",
		IntentType:    IntentSynthesis,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "AI Society", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)

	system := msgs[0].Content
	assert.Contains(t, system, "多面的分析")
}

func TestTemplateRegistry_FallsBackToGeneral(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "something",
		PromptVersion: "alpha-v2",
		IntentType:    IntentType("unknown_intent"),
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
}

// --- Instruction Deduplication ---

func TestTemplateRegistry_NoRedundantJapaneseInstruction(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "テスト",
		PromptVersion: "alpha-v2",
		IntentType:    IntentCausalExplanation,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)

	system := msgs[0].Content
	// "日本語で回答" should appear at most twice (once in instructions, once in sandwich)
	count := strings.Count(system, "日本語")
	assert.LessOrEqual(t, count, 2, "Japanese instruction should not be repeated more than twice, found %d times", count)
}

// --- Token Budget ---

func TestTemplateRegistry_EstimateSystemTokens_Positive(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "テスト",
		PromptVersion: "alpha-v2",
		IntentType:    IntentCausalExplanation,
	}

	tokens := reg.EstimateSystemTokens(input)
	assert.Greater(t, tokens, 0, "system token estimate should be positive")
	assert.Less(t, tokens, 2000, "system token estimate should be reasonable")
}

func TestTemplateRegistry_EstimateSystemTokens_CausalSmallerThanOldBuilder(t *testing.T) {
	reg := NewTemplateRegistry()
	causalInput := PromptInput{
		Query:         "なぜ石油危機が起きた？",
		PromptVersion: "alpha-v2",
		IntentType:    IntentCausalExplanation,
	}

	tokens := reg.EstimateSystemTokens(causalInput)
	// The old builder produced ~1500 tokens for causal. Target: < 900 (60% reduction)
	assert.Less(t, tokens, 900, "causal template should be significantly smaller than old builder")
}

// --- User Message Structure ---

func TestTemplateRegistry_UserMessage_ContainsContextAndQuery(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "テストクエリ",
		PromptVersion: "alpha-v2",
		IntentType:    IntentGeneral,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Article Title", ChunkText: "Article content here"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)

	user := msgs[1].Content
	assert.Contains(t, user, "Article Title")
	assert.Contains(t, user, "Article content here")
	assert.Contains(t, user, "テストクエリ")
}

// --- Multi-turn ---

func TestTemplateRegistry_MultiTurn_PreservesHistory(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "もっと詳しく教えて",
		PromptVersion: "alpha-v2",
		IntentType:    IntentGeneral,
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "前回の質問"},
			{Role: "assistant", Content: "前回の回答"},
		},
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)

	// Should have: system + history messages + user message
	assert.GreaterOrEqual(t, len(msgs), 4, "should include system + 2 history + user")
	assert.Equal(t, "system", msgs[0].Role)
}

// --- Comparison Template ---

func TestTemplateRegistry_ComparisonTemplate(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "AとBの違いは？",
		PromptVersion: "alpha-v2",
		IntentType:    IntentComparison,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)

	system := msgs[0].Content
	assert.Contains(t, system, "比較")
}

// --- FactCheck Template ---

func TestTemplateRegistry_FactCheckTemplate(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "この主張は正しい？",
		PromptVersion: "alpha-v2",
		IntentType:    IntentFactCheck,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)

	system := msgs[0].Content
	assert.Contains(t, system, "主張")
	assert.Contains(t, system, "根拠")
	assert.Contains(t, system, "判定")
}

// --- Temporal Template ---

func TestTemplateRegistry_TemporalTemplate(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "最新の動向は？",
		PromptVersion: "alpha-v2",
		IntentType:    IntentTemporal,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)

	system := msgs[0].Content
	assert.Contains(t, system, "時系列")
}

// --- Article-Scoped Sub-Intent Templates ---

func TestTemplateRegistry_ArticleScoped_CritiqueSubIntent(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "この記事の弱点は？",
		PromptVersion: "alpha-v2",
		IntentType:    IntentArticleScoped,
		SubIntentType: SubIntentCritique,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test Article", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)
	require.Len(t, msgs, 2, "should produce system + user messages")
	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, "user", msgs[1].Role)

	system := msgs[0].Content
	// Must contain critique-specific keywords
	assert.Contains(t, system, "反論")
	assert.Contains(t, system, "弱点")
	// Must NOT contain generic 概要/詳細/まとめ structure
	assert.NotContains(t, system, "## 回答構造")
	assert.NotContains(t, system, "**概要**")
	assert.NotContains(t, system, "**まとめ**")
}

func TestTemplateRegistry_ArticleScoped_RelatedArticlesSubIntent(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "関連する記事はある？",
		PromptVersion: "alpha-v2",
		IntentType:    IntentArticleScoped,
		SubIntentType: SubIntentRelatedArticles,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test Article", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)
	require.Len(t, msgs, 2, "should produce system + user messages")

	system := msgs[0].Content
	// Must contain list-format keywords
	assert.Contains(t, system, "リスト")
	assert.Contains(t, system, "関連")
	// Must NOT contain generic structure
	assert.NotContains(t, system, "## 回答構造")
	assert.NotContains(t, system, "**概要**")
}

func TestTemplateRegistry_ArticleScoped_DetailSubIntent(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "技術的な詳細を教えて",
		PromptVersion: "alpha-v2",
		IntentType:    IntentArticleScoped,
		SubIntentType: SubIntentDetail,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test Article", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)
	require.Len(t, msgs, 2, "should produce system + user messages")

	system := msgs[0].Content
	// Must contain detail-specific keywords
	assert.Contains(t, system, "技術")
	assert.Contains(t, system, "メカニズム")
	// Must NOT contain generic structure
	assert.NotContains(t, system, "## 回答構造")
	assert.NotContains(t, system, "**概要**")
}

func TestTemplateRegistry_ArticleScoped_DefaultSubIntent(t *testing.T) {
	reg := NewTemplateRegistry()
	input := PromptInput{
		Query:         "この記事について教えて",
		PromptVersion: "alpha-v2",
		IntentType:    IntentArticleScoped,
		SubIntentType: SubIntentNone,
		Contexts: []PromptContext{
			{ChunkID: "1", Title: "Test Article", ChunkText: "content"},
		},
	}

	msgs, err := reg.Build(input)
	require.NoError(t, err)
	require.Len(t, msgs, 2, "should produce system + user messages")

	system := msgs[0].Content
	// Default article-scoped should have summary structure
	assert.Contains(t, system, "要点")
	assert.Contains(t, system, "記事")
	// Should NOT use generic 回答構造 section header
	assert.NotContains(t, system, "## 回答構造")
}
