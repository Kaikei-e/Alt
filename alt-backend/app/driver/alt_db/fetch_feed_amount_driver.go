package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *AltDBRepository) FetchFeedAmount(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) FROM feeds
	`

	var amount int
	err := r.pool.QueryRow(ctx, query).Scan(&amount)
	if err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch feed amount", "error", err)
		return 0, errors.New("failed to fetch feed amount")
	}

	logger.SafeInfoContext(ctx, "feed amount fetched successfully", "amount", amount)
	return amount, nil
}
