package recall_rail_usecase

import (
	"alt/domain"
	"alt/port/feature_flag_port"
	"alt/port/recall_candidate_port"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

type RecallRailUsecase struct {
	candidatePort recall_candidate_port.GetRecallCandidatesPort
	featureFlag   feature_flag_port.FeatureFlagPort
	fallbackPort  recall_candidate_port.ArticleFallbackPort
}

func NewRecallRailUsecase(
	candidatePort recall_candidate_port.GetRecallCandidatesPort,
	featureFlag feature_flag_port.FeatureFlagPort,
	fallbackPort recall_candidate_port.ArticleFallbackPort,
) *RecallRailUsecase {
	return &RecallRailUsecase{
		candidatePort: candidatePort,
		featureFlag:   featureFlag,
		fallbackPort:  fallbackPort,
	}
}

func (u *RecallRailUsecase) Execute(ctx context.Context, userID uuid.UUID, limit int) ([]domain.RecallCandidate, error) {
	if u.featureFlag != nil && !u.featureFlag.IsEnabled(domain.FlagRecallRail, userID) {
		return nil, fmt.Errorf("recall rail is not enabled for this user")
	}

	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	candidates, err := u.candidatePort.GetRecallCandidates(ctx, userID, limit)
	if err != nil {
		return nil, err
	}

	if u.fallbackPort != nil {
		u.enrichMissingItems(ctx, candidates)
	}

	return candidates, nil
}

const articlePrefix = "article:"

func (u *RecallRailUsecase) enrichMissingItems(ctx context.Context, candidates []domain.RecallCandidate) {
	for i := range candidates {
		if candidates[i].Item != nil {
			continue
		}
		if !strings.HasPrefix(candidates[i].ItemKey, articlePrefix) {
			continue
		}
		articleID := candidates[i].ItemKey[len(articlePrefix):]
		if _, err := uuid.Parse(articleID); err != nil {
			continue
		}

		title, link, publishedAt, err := u.fallbackPort.GetArticleTitleAndLink(ctx, articleID)
		if err != nil {
			slog.WarnContext(ctx, "recall fallback lookup failed", "article_id", articleID, "error", err)
			continue
		}
		if title == "" {
			continue
		}

		aid := uuid.MustParse(articleID)
		candidates[i].Item = &domain.KnowledgeHomeItem{
			ItemKey:      candidates[i].ItemKey,
			ItemType:     domain.ItemArticle,
			PrimaryRefID: &aid,
			Title:        title,
			Link:         link,
			PublishedAt:  publishedAt,
			SummaryState: domain.SummaryStateMissing,
		}
	}
}
