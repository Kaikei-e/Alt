package recall_rail_usecase

import (
	"alt/domain"
	"alt/port/feature_flag_port"
	"alt/port/recall_candidate_port"
	"context"
	"fmt"

	"github.com/google/uuid"
)

type RecallRailUsecase struct {
	candidatePort recall_candidate_port.GetRecallCandidatesPort
	featureFlag   feature_flag_port.FeatureFlagPort
}

func NewRecallRailUsecase(
	candidatePort recall_candidate_port.GetRecallCandidatesPort,
	featureFlag feature_flag_port.FeatureFlagPort,
) *RecallRailUsecase {
	return &RecallRailUsecase{
		candidatePort: candidatePort,
		featureFlag:   featureFlag,
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

	return u.candidatePort.GetRecallCandidates(ctx, userID, limit)
}
