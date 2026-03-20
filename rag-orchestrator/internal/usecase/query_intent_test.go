package usecase

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestParseQueryIntent_GeneralQuery(t *testing.T) {
	raw := "What is the latest trend in AI?"
	intent := ParseQueryIntent(raw)

	assert.Equal(t, IntentGeneral, intent.IntentType)
	assert.Equal(t, raw, intent.UserQuestion)
	assert.Equal(t, raw, intent.OriginalQuery)
	assert.Empty(t, intent.ArticleID)
	assert.Empty(t, intent.ArticleTitle)
}

func TestParseQueryIntent_ArticleScoped(t *testing.T) {
	raw := "Regarding the article: OpenAI releases GPT-5 [articleId: abc-123]\n\nQuestion:\nWhat are the key improvements?"

	intent := ParseQueryIntent(raw)

	assert.Equal(t, IntentArticleScoped, intent.IntentType)
	assert.Equal(t, "abc-123", intent.ArticleID)
	assert.Equal(t, "OpenAI releases GPT-5", intent.ArticleTitle)
	assert.Equal(t, "What are the key improvements?", intent.UserQuestion)
	assert.Equal(t, raw, intent.OriginalQuery)
}

func TestParseQueryIntent_ArticleScopedWithRealUUID(t *testing.T) {
	raw := "Regarding the article: AI Industry Update [articleId: b275e2cb-04cc-47f6-a1cd-0bd4e6a5c953]\n\nQuestion:\nSummarize the main points."

	intent := ParseQueryIntent(raw)

	assert.Equal(t, IntentArticleScoped, intent.IntentType)
	assert.Equal(t, "b275e2cb-04cc-47f6-a1cd-0bd4e6a5c953", intent.ArticleID)
	assert.Equal(t, "AI Industry Update", intent.ArticleTitle)
	assert.Equal(t, "Summarize the main points.", intent.UserQuestion)
}

func TestParseQueryIntent_MalformedRef(t *testing.T) {
	raw := "Regarding the article: Some Title\n\nQuestion:\nWhat is this?"

	intent := ParseQueryIntent(raw)

	assert.Equal(t, IntentGeneral, intent.IntentType)
	assert.Equal(t, "What is this?", intent.UserQuestion)
	assert.Empty(t, intent.ArticleID)
}

func TestParseQueryIntent_ContextFormat(t *testing.T) {
	raw := "Context:\nSome context here\n\nQuestion:\nWhat does this mean?"

	intent := ParseQueryIntent(raw)

	assert.Equal(t, IntentGeneral, intent.IntentType)
	assert.Equal(t, raw, intent.UserQuestion)
}

func TestParseQueryIntent_TitleWithBrackets(t *testing.T) {
	raw := "Regarding the article: [Breaking] New AI Model [v2.0] Released [articleId: xyz-789]\n\nQuestion:\nWhat changed?"

	intent := ParseQueryIntent(raw)

	assert.Equal(t, IntentArticleScoped, intent.IntentType)
	assert.Equal(t, "xyz-789", intent.ArticleID)
	assert.Equal(t, "[Breaking] New AI Model [v2.0] Released", intent.ArticleTitle)
	assert.Equal(t, "What changed?", intent.UserQuestion)
}

func TestParseQueryIntent_EmptyQuestion(t *testing.T) {
	raw := "Regarding the article: Some Title [articleId: abc-123]\n\nQuestion:\n"

	intent := ParseQueryIntent(raw)

	assert.Equal(t, IntentArticleScoped, intent.IntentType)
	assert.Equal(t, "abc-123", intent.ArticleID)
	assert.Equal(t, "", intent.UserQuestion)
}

func TestParseQueryIntent_QuestionContainsQuestionKeyword(t *testing.T) {
	raw := "Regarding the article: FAQ Guide [articleId: faq-001]\n\nQuestion:\nThe section titled \"Question:\" is confusing. Can you explain?"

	intent := ParseQueryIntent(raw)

	assert.Equal(t, IntentArticleScoped, intent.IntentType)
	assert.Equal(t, "faq-001", intent.ArticleID)
	assert.Equal(t, "FAQ Guide", intent.ArticleTitle)
	// LastIndex picks the last "\n\nQuestion:\n" separator
	assert.Equal(t, raw, intent.OriginalQuery)
}

func TestResolveQueryIntent_UsesCurrentArticleScopeWhenPresent(t *testing.T) {
	raw := "Regarding the article: Current Title [articleId: art-current]\n\nQuestion:\nWhat changed?"
	history := []domain.Message{
		{Role: "user", Content: "Regarding the article: Older Title [articleId: art-old]\n\nQuestion:\nSummarize"},
	}

	intent := ResolveQueryIntent(raw, history)

	assert.Equal(t, IntentArticleScoped, intent.IntentType)
	assert.Equal(t, "art-current", intent.ArticleID)
	assert.Equal(t, "Current Title", intent.ArticleTitle)
	assert.Equal(t, "What changed?", intent.UserQuestion)
}

func TestResolveQueryIntent_InheritsArticleScopeFromHistory(t *testing.T) {
	raw := "What is the impact?"
	history := []domain.Message{
		{Role: "assistant", Content: "Previous answer"},
		{Role: "user", Content: "Regarding the article: OpenAI GPT-5 [articleId: art-123]\n\nQuestion:\nWhat changed?"},
	}

	intent := ResolveQueryIntent(raw, history)

	assert.Equal(t, IntentArticleScoped, intent.IntentType)
	assert.Equal(t, "art-123", intent.ArticleID)
	assert.Equal(t, "OpenAI GPT-5", intent.ArticleTitle)
	assert.Equal(t, "What is the impact?", intent.UserQuestion)
	assert.Equal(t, raw, intent.OriginalQuery)
}
