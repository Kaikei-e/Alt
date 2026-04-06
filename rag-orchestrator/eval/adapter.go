package eval

import (
	"context"
	"unicode/utf8"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
)

// PipelineAdapter wraps the RAG pipeline to produce EvalResults from GoldenCases.
type PipelineAdapter struct {
	answerUsecase usecase.AnswerWithRAGUsecase
}

// NewPipelineAdapter creates an adapter that runs golden cases through the live pipeline.
func NewPipelineAdapter(uc usecase.AnswerWithRAGUsecase) *PipelineAdapter {
	return &PipelineAdapter{answerUsecase: uc}
}

// RunCase executes a single golden case through the pipeline and returns an EvalResult.
func (a *PipelineAdapter) RunCase(ctx context.Context, gc GoldenCase) EvalResult {
	result := EvalResult{CaseID: gc.ID}

	input := usecase.AnswerWithRAGInput{
		Query:  gc.Query,
		Locale: "ja",
	}

	// Convert conversation history
	for _, msg := range gc.ConversationHistory {
		input.ConversationHistory = append(input.ConversationHistory, domain.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Article scope
	if gc.ArticleScope != nil {
		input.CandidateArticleIDs = []string{gc.ArticleScope.ArticleID}
		// Prefix query with article metadata (matches ParseQueryIntent format)
		input.Query = "Regarding the article: " + gc.ArticleScope.Title +
			" [articleId: " + gc.ArticleScope.ArticleID + "]\n\nQuestion:\n" + gc.Query
	}

	output, err := a.answerUsecase.Execute(ctx, input)
	if err != nil {
		result.IsFallback = true
		result.FallbackReason = err.Error()
		return result
	}

	// Map output to EvalResult
	result.Answer = output.Answer
	result.AnswerLength = utf8.RuneCountInString(output.Answer)
	result.IsFallback = output.Fallback
	result.FallbackReason = output.Reason
	result.CitationCount = len(output.Citations)
	result.QualityFlags = output.Debug.QualityFlags

	for _, ctx := range output.Contexts {
		result.RetrievedTitles = append(result.RetrievedTitles, ctx.Title)
		result.RetrievedScores = append(result.RetrievedScores, ctx.Score)
	}

	for _, cite := range output.Citations {
		result.CitedTitles = append(result.CitedTitles, cite.Title)
	}

	result.ExpandedQueries = output.Debug.ExpandedQueries
	result.IntentClassified = output.Debug.IntentType
	result.RetrievalPolicy = output.Debug.RetrievalPolicy
	result.PlannerConfidence = output.Debug.PlannerConfidence
	result.ClarificationAsked = output.Debug.NeedsClarification
	result.BM25HitCount = output.Debug.BM25HitCount

	return result
}

// RunAll executes all golden cases and returns results keyed by case ID.
func (a *PipelineAdapter) RunAll(ctx context.Context, cases []GoldenCase) map[string]EvalResult {
	results := make(map[string]EvalResult, len(cases))
	for _, gc := range cases {
		results[gc.ID] = a.RunCase(ctx, gc)
	}
	return results
}
