package usecase

import (
	"context"
	"log/slog"
	"strings"
)

type causalStrategy struct {
	retrieve        RetrieveContextUsecase
	qualityAssessor *RetrievalQualityAssessor
	logger          *slog.Logger
}

// NewCausalStrategy creates a retrieval strategy optimized for causal queries.
// It decomposes the original question into focused subqueries and keeps the
// best result, stopping early when a good retrieval is found.
func NewCausalStrategy(retrieve RetrieveContextUsecase, qualityAssessor *RetrievalQualityAssessor, logger *slog.Logger) RetrievalStrategy {
	return &causalStrategy{
		retrieve:        retrieve,
		qualityAssessor: qualityAssessor,
		logger:          logger,
	}
}

func (s *causalStrategy) Name() string { return "causal" }

func (s *causalStrategy) Retrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	baseQuery := strings.TrimSpace(input.Query)
	if baseQuery == "" {
		baseQuery = strings.TrimSpace(intent.UserQuestion)
	}
	requireGoodVerdict := queryRequestsDetailedAnswer(baseQuery) || intent.SubIntentType == SubIntentDetail

	s.log("causal_strategy_execute",
		slog.String("query", baseQuery),
		slog.String("intent", string(intent.IntentType)),
		slog.Bool("require_good_verdict", requireGoodVerdict))

	queries := causalSubqueries(baseQuery)
	var (
		bestOutput  *RetrieveContextOutput
		bestVerdict QualityVerdict
		bestQuery   string
	)

	for i, query := range queries {
		retryInput := input
		retryInput.Query = query

		s.log("causal_strategy_attempt",
			slog.Int("attempt", i+1),
			slog.String("query", query))

		output, err := s.retrieve.Execute(ctx, retryInput)
		if err != nil {
			s.log("causal_strategy_attempt_failed",
				slog.Int("attempt", i+1),
				slog.String("query", query),
				slog.String("error", err.Error()))
			continue
		}
		if output == nil || len(output.Contexts) == 0 {
			continue
		}

		verdict := QualityGood
		if s.qualityAssessor != nil {
			verdict = s.qualityAssessor.AssessWithIntent(output.Contexts, intent.IntentType, intent.UserQuestion)
		}

		if bestOutput == nil || qualityRank(verdict) > qualityRank(bestVerdict) || (verdict == bestVerdict && len(output.Contexts) > len(bestOutput.Contexts)) {
			bestOutput = output
			bestVerdict = verdict
			bestQuery = query
		}

		if verdict == QualityGood {
			s.log("causal_strategy_early_exit",
				slog.Int("attempt", i+1),
				slog.String("query", query),
				slog.Int("contexts", len(output.Contexts)))
			return output, nil
		}
	}

	if bestOutput != nil {
		if requireGoodVerdict && bestVerdict != QualityGood {
			s.log("causal_strategy_rejected_marginal_result",
				slog.String("query", bestQuery),
				slog.String("verdict", string(bestVerdict)),
				slog.Int("contexts", len(bestOutput.Contexts)))
			return nil, nil
		}
		s.log("causal_strategy_selected",
			slog.String("query", bestQuery),
			slog.String("verdict", string(bestVerdict)),
			slog.Int("contexts", len(bestOutput.Contexts)))
		return bestOutput, nil
	}

	s.log("causal_strategy_empty",
		slog.String("query", baseQuery))
	return nil, nil
}

func causalSubqueries(baseQuery string) []string {
	if strings.TrimSpace(baseQuery) == "" {
		return nil
	}
	return []string{
		baseQuery,
		baseQuery + " 供給 制裁 sanctions supply",
		baseQuery + " 地政学 geopolitical conflict",
		baseQuery + " 経済 market price impact",
	}
}

func qualityRank(verdict QualityVerdict) int {
	switch verdict {
	case QualityGood:
		return 3
	case QualityMarginal:
		return 2
	case QualityInsufficient:
		return 1
	default:
		return 0
	}
}

func (s *causalStrategy) log(msg string, attrs ...slog.Attr) {
	if s.logger == nil {
		return
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	s.logger.Info(msg, args...)
}
