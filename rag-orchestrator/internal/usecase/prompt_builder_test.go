package usecase_test

import (
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptBuilder_MultiTurnReturnsChatMessages(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "もっと詳しく教えて",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content about the topic"},
		},
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "この記事の要点は？"},
			{Role: "assistant", Content: "この記事は新しいプロトコルについて説明しています。"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	// Multi-turn: system + history + current user
	assert.GreaterOrEqual(t, len(msgs), 4,
		"multi-turn should produce system + history + current user messages")

	// First message should be system with instructions
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "あなたの役割",
		"system message should contain role instructions")

	// Past turns should be actual chat messages (shifted by +1 due to system)
	assert.Equal(t, "user", msgs[1].Role)
	assert.Contains(t, msgs[1].Content, "この記事の要点は？")
	assert.Equal(t, "assistant", msgs[2].Role)
	assert.Contains(t, msgs[2].Content, "新しいプロトコルについて")

	// Last message should be user with context + query (no instructions)
	lastMsg := msgs[len(msgs)-1]
	assert.Equal(t, "user", lastMsg.Role)
	assert.Contains(t, lastMsg.Content, "もっと詳しく教えて")
	assert.Contains(t, lastMsg.Content, "Content about the topic",
		"current turn should include context chunks")
}

func TestPromptBuilder_MultiTurnFollowUpInstructions(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "深掘りして",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "要点は？"},
			{Role: "assistant", Content: "回答テキスト"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	// Follow-up instructions should be in the system message, not the user message
	sysMsg := msgs[0]
	assert.Equal(t, "system", sysMsg.Role)
	assert.Contains(t, sysMsg.Content, "繰り返さない",
		"system message should instruct not to repeat")
	assert.Contains(t, sysMsg.Content, "直接回答",
		"system message should request direct answer without overview")
	assert.Contains(t, sysMsg.Content, "日本語",
		"system message should enforce Japanese response")

	// User message should have only context + query, not instructions
	lastMsg := msgs[len(msgs)-1]
	assert.Equal(t, "user", lastMsg.Role)
	assert.NotContains(t, lastMsg.Content, "繰り返さない",
		"user message should not contain follow-up instructions")
	assert.NotContains(t, lastMsg.Content, "## 概要\\n...\\n## 詳細",
		"follow-up should NOT include overview/detail/summary format")
}

func TestPromptBuilder_SingleTurnUnchanged(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "この記事の要点は？",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: nil,
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	// Single-turn should return system + user messages
	assert.Len(t, msgs, 2, "single-turn should return system + user messages")
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "あなたの役割",
		"system message should include full instructions")
	assert.Equal(t, "user", msgs[1].Role)
	assert.Contains(t, msgs[1].Content, "この記事の要点は？",
		"user message should contain the query")
}

// --- SubIntent prompt tests ---

func TestPromptBuilder_SingleTurn_CritiqueSubIntent_NoSummaryStructure(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "反論はある？",
		Locale:        "ja",
		PromptVersion: "v1",
		IntentType:    usecase.IntentArticleScoped,
		SubIntentType: usecase.SubIntentCritique,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test Article", ChunkText: "Article content here"},
		},
		ArticleContext: &usecase.ArticleContext{
			ArticleID: "art-123",
			Title:     "Test Article",
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)
	assert.Len(t, msgs, 2, "single-turn should return system + user messages")

	sysContent := msgs[0].Content
	assert.Equal(t, "system", msgs[0].Role)
	// Should contain critique-specific instructions in system message
	assert.Contains(t, sysContent, "批判的分析")
	assert.Contains(t, sysContent, "反論")
	// answer exemplar should NOT contain summary template structure
	assert.NotContains(t, sysContent, "## 概要\\n...\\n## 詳細\\n...\\n## まとめ")
	// Should contain analytical output format instruction
	assert.Contains(t, sysContent, "質問に対する分析的な回答")
	// Schema (JSON structure) should still be present
	assert.Contains(t, sysContent, "\"answer\"")
	assert.Contains(t, sysContent, "\"citations\"")

	// User message should contain context and query
	assert.Equal(t, "user", msgs[1].Role)
	assert.Contains(t, msgs[1].Content, "Article content here")
}

func TestPromptBuilder_SingleTurn_NoSubIntent_KeepsSummaryStructure(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "この記事の要点は？",
		Locale:        "ja",
		PromptVersion: "v1",
		IntentType:    usecase.IntentArticleScoped,
		SubIntentType: usecase.SubIntentNone,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)
	// Default behavior preserved: summary structure present
	assert.Contains(t, msgs[0].Content, "概要")
	assert.Contains(t, msgs[0].Content, "詳細")
	assert.Contains(t, msgs[0].Content, "まとめ")
}

func TestPromptBuilder_MultiTurn_CritiqueSubIntent_AddsGuidance(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "反論はある？",
		Locale:        "ja",
		PromptVersion: "v1",
		SubIntentType: usecase.SubIntentCritique,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "この記事について教えて"},
			{Role: "assistant", Content: "この記事は..."},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	// Multi-turn critique guidance should be in system message
	sysMsg := msgs[0]
	assert.Equal(t, "system", sysMsg.Role)
	assert.Contains(t, sysMsg.Content, "弱点")
	assert.Contains(t, sysMsg.Content, "反証")
	// Should still have "don't repeat" in system message
	assert.Contains(t, sysMsg.Content, "繰り返さない")

	// User message should have only context + query
	lastMsg := msgs[len(msgs)-1]
	assert.Equal(t, "user", lastMsg.Role)
}

func TestPromptBuilder_SingleTurn_AnalyticalRelaxes800CharRule(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "反論はある？",
		Locale:        "ja",
		PromptVersion: "v1",
		SubIntentType: usecase.SubIntentCritique,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)
	// Analytical queries should NOT force 800-char minimum
	assert.NotContains(t, msgs[0].Content, "800文字以上")
}

func TestPromptBuilder_NoHistoryOmitsConversationSection(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "この記事の要点は？",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: nil,
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	assert.NotContains(t, msgs[0].Content, "会話履歴",
		"prompt without history should not contain conversation history section")
	assert.NotContains(t, msgs[0].Content, "フォローアップ",
		"prompt without history should not contain follow-up instructions")
}

// --- Phase E: PlannerOutput-driven prompt tests ---

func TestPromptBuilder_WithPlannerOutput_Detail_NoSummaryStructure(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "技術的な詳細をもっと教えて",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content about the topic"},
		},
		IntentType:    usecase.IntentArticleScoped,
		SubIntentType: usecase.SubIntentDetail,
		PlannerOutput: &domain.PlannerOutput{
			Operation:       domain.OpDetail,
			RetrievalPolicy: domain.PolicyArticleOnly,
			Confidence:      0.85,
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	content := msgs[0].Content
	// Detail operation should NOT include summary structure
	assert.NotContains(t, content, "## 概要",
		"detail operation should not include summary-style 概要 section")
	assert.Contains(t, content, "技術的",
		"detail operation should mention technical focus")
}

func TestPromptBuilder_CausalExplanation_HasStructuredRequirement(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "最近の石油危機の真因は？",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Oil Crisis", ChunkText: "Crisis content"},
		},
		IntentType: usecase.IntentCausalExplanation,
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	content := msgs[0].Content
	assert.Contains(t, content, "直接的要因")
	assert.Contains(t, content, "構造的背景")
	assert.Contains(t, content, "不確実性")
	assert.Contains(t, content, "単一の原因に帰結させず")
}

func TestPromptBuilder_SubIntent_OmitsGenericAnswerStructure(t *testing.T) {
	// When a SubIntent is detected, the generic 回答構造 section (概要/詳細/まとめ)
	// should NOT be included in instructions because it conflicts with the
	// SubIntent-specific guidance (e.g., "批判的分析" vs "概要を述べる").
	tests := []struct {
		name      string
		subIntent usecase.SubIntentType
	}{
		{"critique", usecase.SubIntentCritique},
		{"detail", usecase.SubIntentDetail},
		{"implication", usecase.SubIntentImplication},
		{"evidence", usecase.SubIntentEvidence},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := usecase.NewXMLPromptBuilder()
			input := usecase.PromptInput{
				Query:         "テスト質問",
				Locale:        "ja",
				PromptVersion: "v1",
				IntentType:    usecase.IntentArticleScoped,
				SubIntentType: tt.subIntent,
				Contexts: []usecase.PromptContext{
					{ChunkID: "1", Title: "Test", ChunkText: "Content"},
				},
			}

			msgs, err := builder.Build(input)
			require.NoError(t, err)

			content := msgs[0].Content
			assert.NotContains(t, content, "## 回答構造",
				"SubIntent %s should not include generic 回答構造 section", tt.subIntent)
		})
	}
}

func TestPromptBuilder_NoSubIntent_KeepsGenericAnswerStructure(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "この記事について教えて",
		Locale:        "ja",
		PromptVersion: "v1",
		IntentType:    usecase.IntentGeneral,
		SubIntentType: usecase.SubIntentNone,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)
	assert.Contains(t, msgs[0].Content, "## 回答構造",
		"generic queries should include 回答構造 section")
}

func TestPromptBuilder_WithPlannerOutput_General_KeepsSummaryStructure(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "AIの最新動向は？",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "AI News", ChunkText: "AI trends content"},
		},
		IntentType: usecase.IntentGeneral,
		PlannerOutput: &domain.PlannerOutput{
			Operation:       domain.OpGeneral,
			RetrievalPolicy: domain.PolicyGlobalOnly,
			Confidence:      0.8,
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	content := msgs[0].Content
	// General operation should keep summary structure
	assert.Contains(t, content, "概要",
		"general operation should include 概要 in output format")
}

func TestPromptBuilder_IntentSynthesis_MultiAspectStructure(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "ニューヨークと芸術のかかわり",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "NYC Art", ChunkText: "Art in New York"},
		},
		IntentType: usecase.IntentSynthesis,
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	content := msgs[0].Content
	assert.Contains(t, content, "概念的合成",
		"synthesis should include 概念的合成 intent section")
	assert.Contains(t, content, "多面的分析",
		"synthesis should require multi-aspect analysis")
	assert.Contains(t, content, "相互関係",
		"synthesis should require relationship analysis")
	assert.Contains(t, content, "1200文字以上",
		"synthesis should require 1200+ characters")
}

func TestPromptBuilder_IntentSynthesis_SkipsGenericStructure(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "AIと教育の関係",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "AI Education", ChunkText: "AI in education"},
		},
		IntentType: usecase.IntentSynthesis,
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	content := msgs[0].Content
	assert.NotContains(t, content, "## 回答構造",
		"synthesis intent should NOT include generic 回答構造 section (概要/詳細/まとめ)")
}

// --- System role separation tests ---

func TestPromptBuilder_SingleTurn_SystemMessageContainsInstructions(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "テスト質問",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	sysContent := msgs[0].Content
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, sysContent, "あなたの役割",
		"system message should contain role definition")
	assert.Contains(t, sysContent, "品質基準",
		"system message should contain quality criteria")
	assert.Contains(t, sysContent, "出力形式",
		"system message should contain output format")
	assert.Contains(t, sysContent, "\"answer\"",
		"system message should contain JSON schema")
}

func TestPromptBuilder_SingleTurn_UserMessageContainsOnlyContextAndQuery(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "テスト質問",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test Article", ChunkText: "Article text content"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	userContent := msgs[1].Content
	assert.Equal(t, "user", msgs[1].Role)
	// User message should contain context and query
	assert.Contains(t, userContent, "Article text content",
		"user message should contain context chunks")
	assert.Contains(t, userContent, "テスト質問",
		"user message should contain the query")
	// User message should NOT contain instructions
	assert.NotContains(t, userContent, "あなたの役割",
		"user message should not contain role definition")
	assert.NotContains(t, userContent, "品質基準",
		"user message should not contain quality criteria")
	assert.NotContains(t, userContent, "出力形式",
		"user message should not contain output format")
}

func TestPromptBuilder_SingleTurn_InstructionSandwich(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "テスト質問",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	sysContent := msgs[0].Content
	assert.Contains(t, sysContent, "重要な注意",
		"system message should end with instruction sandwich reminder")
}

func TestPromptBuilder_SingleTurn_FewShotExample(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "テスト質問",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	sysContent := msgs[0].Content
	assert.Contains(t, sysContent, "<example>",
		"system message should contain a few-shot example")
}

func TestPromptBuilder_MultiTurn_SystemMessageReinjected(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "続きを教えて",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "最初の質問"},
			{Role: "assistant", Content: "最初の回答"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	// System message should be re-injected with core instructions
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "あなたの役割",
		"multi-turn system message should re-inject core instructions")
	assert.Contains(t, msgs[0].Content, "品質基準",
		"multi-turn system message should re-inject quality criteria")
}

func TestPromptBuilder_MultiTurn_UserMessageHasOnlyContextAndQuery(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "詳しく教えて",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "最初の質問"},
			{Role: "assistant", Content: "最初の回答"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	lastMsg := msgs[len(msgs)-1]
	assert.Equal(t, "user", lastMsg.Role)
	assert.NotContains(t, lastMsg.Content, "フォローアップ指示",
		"user message should not contain follow-up instruction header")
	assert.NotContains(t, lastMsg.Content, "あなたの役割",
		"user message should not contain role definition")
	assert.Contains(t, lastMsg.Content, "詳しく教えて",
		"user message should contain query")
}

func TestPromptBuilder_MultiTurn_SystemContainsFollowUpRules(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "もっと教えて",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "最初の質問"},
			{Role: "assistant", Content: "最初の回答"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	sysContent := msgs[0].Content
	assert.Contains(t, sysContent, "繰り返さない",
		"system message should contain no-repeat rule")
	assert.Contains(t, sysContent, "新しい事実",
		"system message should instruct new facts only")
	assert.Contains(t, sysContent, "重要な注意",
		"system message should contain instruction sandwich")
}
