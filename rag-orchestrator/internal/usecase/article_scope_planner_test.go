package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// articleScopedQuery reproduces the envelope alt-frontend-sv's AskSheet sends
// for a question asked from an article page.
func articleScopedQuery(articleID, title, question string) string {
	return "Regarding the article: " + title + " [articleId: " + articleID + "]" +
		"\n\nQuestion:\n" + question
}

func answerJSON(text string) string {
	return `{"answer":"` + text + `","citations":[{"chunk_id":"1","reason":"r"}],"fallback":false,"reason":""}`
}

// The query planner's intent vocabulary (domain.QueryPlan.Intent) has no
// article_scoped member, so mapping result.intentType straight off the plan
// meant selectStrategy could never return the article-scoped strategy. The
// article the user is reading was dropped from retrieval on every turn, while
// the debug payload still claimed retrieval_policy=article_only.
func TestBuildPrompt_ArticleScopedSurvivesPlannerIntent(t *testing.T) {
	articleID := uuid.New().String()

	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	mockQP := new(mockQueryPlannerPort)
	articleStrategy := &mockRetrievalStrategy{name: "article_scoped"}
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	mockQP.On("PlanQuery", mock.Anything, mock.Anything).Return(&domain.QueryPlan{
		ResolvedQuery:   "この記事の要点は何ですか",
		SearchQueries:   []string{"要点"},
		Intent:          "general",
		RetrievalPolicy: "article_only",
		AnswerFormat:    "summary",
	}, nil)

	articleStrategy.On("Retrieve", mock.Anything, mock.Anything, mock.Anything).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{{
				ChunkText: "記事本文のチャンク。供給網の逼迫について述べている。",
				Title:     "Supply chain report",
				URL:       "https://example.com/a",
				ArticleID: articleID,
				Score:     1.0,
				ChunkID:   uuid.New(),
			}},
		}, nil)
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).
		Return(&usecase.RetrieveContextOutput{}, nil)
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: answerJSON("記事の要点は供給網の逼迫です。[1]")}, nil)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve,
		usecase.NewXMLPromptBuilder(),
		mockLLM,
		usecase.NewOutputValidator(5),
		7, 512, 6000,
		"v1", "ja",
		testLogger,
		usecase.WithQueryPlanner(mockQP),
		usecase.WithStrategy(usecase.IntentArticleScoped, articleStrategy),
	)

	out, err := uc.Execute(context.Background(), usecase.AnswerWithRAGInput{
		Query: articleScopedQuery(articleID, "Supply chain report", "この記事の要点は何ですか"),
	})
	require.NoError(t, err)
	require.NotNil(t, out)

	articleStrategy.AssertCalled(t, "Retrieve", mock.Anything, mock.Anything, mock.Anything)
	assert.Equal(t, string(usecase.IntentArticleScoped), out.Debug.IntentType,
		"article scope comes from the request envelope, not from the planner's guess")
	assert.Equal(t, "article_scoped", out.Debug.StrategyUsed)
}

// Even when the strategy that runs is not the article-scoped one, the article
// has to reach retrieval as a candidate. Without this the "article_only"
// policy label sat on top of a completely unscoped global search.
func TestBuildPrompt_ArticleScopedPassesCandidateArticleID(t *testing.T) {
	articleID := uuid.New().String()

	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	mockQP := new(mockQueryPlannerPort)
	articleStrategy := &mockRetrievalStrategy{name: "article_scoped"}
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	mockQP.On("PlanQuery", mock.Anything, mock.Anything).Return(&domain.QueryPlan{
		ResolvedQuery:   "この記事の背景",
		Intent:          "topic_deep_dive",
		RetrievalPolicy: "global_only",
		AnswerFormat:    "detail",
	}, nil)

	var seen usecase.RetrieveContextInput
	articleStrategy.On("Retrieve", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			seen = args.Get(1).(usecase.RetrieveContextInput)
		}).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{{
				ChunkText: "背景の説明チャンク。",
				Title:     "Supply chain report",
				ArticleID: articleID,
				Score:     1.0,
				ChunkID:   uuid.New(),
			}},
		}, nil)
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).
		Return(&usecase.RetrieveContextOutput{}, nil)
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: answerJSON("背景は供給網の逼迫です。[1]")}, nil)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve,
		usecase.NewXMLPromptBuilder(),
		mockLLM,
		usecase.NewOutputValidator(5),
		7, 512, 6000,
		"v1", "ja",
		testLogger,
		usecase.WithQueryPlanner(mockQP),
		usecase.WithStrategy(usecase.IntentArticleScoped, articleStrategy),
	)

	_, err := uc.Execute(context.Background(), usecase.AnswerWithRAGInput{
		Query: articleScopedQuery(articleID, "Supply chain report", "この記事の背景"),
	})
	require.NoError(t, err)

	assert.Contains(t, seen.CandidateArticleIDs, articleID,
		"the article under discussion must be in the candidate set regardless of planner policy")
}

// The legacy path degrades gracefully when the article is not indexed; the
// planner path used to turn the same condition into a hard fallback answer.
func TestBuildPrompt_ArticleScopedNotIndexed_FallsBackToConstrainedGeneral(t *testing.T) {
	articleID := uuid.New().String()

	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	mockQP := new(mockQueryPlannerPort)
	articleStrategy := &mockRetrievalStrategy{name: "article_scoped"}
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	mockQP.On("PlanQuery", mock.Anything, mock.Anything).Return(&domain.QueryPlan{
		ResolvedQuery:   "この記事の要点",
		Intent:          "general",
		RetrievalPolicy: "article_only",
		AnswerFormat:    "summary",
	}, nil)

	articleStrategy.On("Retrieve", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, usecase.ErrArticleNotIndexed)
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{{
				ChunkText: "一般検索で見つかったチャンク。",
				Title:     "Fallback source",
				ArticleID: articleID,
				Score:     0.6,
				ChunkID:   uuid.New(),
			}},
		}, nil)
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: answerJSON("要点は供給網の逼迫です。[1]")}, nil)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve,
		usecase.NewXMLPromptBuilder(),
		mockLLM,
		usecase.NewOutputValidator(5),
		7, 512, 6000,
		"v1", "ja",
		testLogger,
		usecase.WithQueryPlanner(mockQP),
		usecase.WithStrategy(usecase.IntentArticleScoped, articleStrategy),
	)

	out, err := uc.Execute(context.Background(), usecase.AnswerWithRAGInput{
		Query: articleScopedQuery(articleID, "Supply chain report", "この記事の要点"),
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.False(t, out.Fallback, "an unindexed article must degrade to constrained general retrieval, not a fallback answer")
	assert.Equal(t, "article_constrained_fallback", out.Debug.StrategyUsed)
}
