package feeds_in_window_usecase

import (
	"alt/dataplane/port/feeds_in_window_port"
	"alt/domain"
	"context"
	"errors"
	"fmt"
	"time"
)

// Config carries tunables for pagination and date window validation.
type Config struct {
	DefaultPageSize int
	MaxPageSize     int
	MaxRangeDays    int
}

// Input encapsulates the filters accepted by the usecase.
type Input struct {
	From     time.Time
	To       time.Time
	Page     int
	PageSize int
}

// FeedsInWindowUsecase validates feeds in window queries before delegating to storage.
type FeedsInWindowUsecase struct {
	repo feeds_in_window_port.FeedsInWindowPort
	cfg  Config
}

// NewFeedsInWindowUsecase builds a new usecase instance.
func NewFeedsInWindowUsecase(repo feeds_in_window_port.FeedsInWindowPort, cfg Config) *FeedsInWindowUsecase {
	return &FeedsInWindowUsecase{repo: repo, cfg: cfg}
}

// Execute returns paginated RSS feed items for the requested time window.
func (u *FeedsInWindowUsecase) Execute(ctx context.Context, input Input) (*domain.FeedsInWindowPage, error) {
	if input.To.IsZero() || input.From.IsZero() {
		return nil, errors.New("from/to parameters are required")
	}

	if !input.From.Before(input.To) {
		return nil, fmt.Errorf("from must be before to")
	}

	if u.cfg.MaxRangeDays > 0 {
		maxRange := time.Duration(u.cfg.MaxRangeDays) * 24 * time.Hour
		if input.To.Sub(input.From) > maxRange {
			return nil, fmt.Errorf("date range exceeds %d days", u.cfg.MaxRangeDays)
		}
	}

	page := input.Page
	if page == 0 {
		page = 1
	}
	if page < 1 {
		return nil, fmt.Errorf("page must be >= 1")
	}

	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = u.cfg.DefaultPageSize
		if pageSize == 0 {
			pageSize = 500
		}
	}
	if pageSize < 1 {
		pageSize = 1
	}
	maxPageSize := u.cfg.MaxPageSize
	if maxPageSize <= 0 {
		maxPageSize = 1000
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	query := domain.FeedsInWindowQuery{
		From:     input.From,
		To:       input.To,
		Page:     page,
		PageSize: pageSize,
	}

	res, err := u.repo.FetchFeedsInWindow(ctx, query)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return &domain.FeedsInWindowPage{
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	res.HasMore = res.Page*res.PageSize < res.Total
	return res, nil
}
